package kafkarep

import (
	"chat-system/core/repo"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	otelkafkakonsumer "github.com/Trendyol/otel-kafka-konsumer"
	"github.com/kamva/mgm/v3"
	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

type ReaderConf struct {
	KafkaHost string        `env:"KAFKA_HOST"`
	Topic     string        `env:"KAFKA_TOPIC" default:"chat-messages"`
	MaxBytes  int           `env:"READER_MAX_BYTES" default:"2000000"` // 2MB
	MaxWait   time.Duration `env:"READER_MAX_WAIT" default:"2s"`
	GroupID   string        `env:"READER_GROUP_ID" default:"chat-messages-mongo-connect"`
}

func NewInsecureReader(conf *ReaderConf) *kafka.Reader {
	kafkaConf := kafka.ReaderConfig{
		Brokers:  []string{conf.KafkaHost},
		Topic:    conf.Topic,
		MaxBytes: conf.MaxBytes,
		GroupID:  conf.GroupID,
		MaxWait:  conf.MaxWait,
		MinBytes: 1,
	}
	err := kafkaConf.Validate()
	if err != nil {
		panic(err)
	}

	kafkaReader := kafka.NewReader(kafkaConf)
	return kafkaReader
}

type mongoAggr struct {
	mgm.IDField `bson:",inline"`
	Topic       string             `bson:"topicID"`
	MinId       primitive.ObjectID `bson:"minID"`
	MaxId       primitive.ObjectID `bson:"maxID"`
	Len         int                `bson:"size"`
	Messages    []repo.Message     `bson:"messages"`
}

type MongoTransactionErr struct {
	error
}

// MongoConnect is responsible for reading messages from Kafka
// and write messages to MongoDB.
type MongoConnect struct {
	reader   *otelkafkakonsumer.Reader
	mongoCli *mongo.Client
	coll     mgm.Collection
	ctx      context.Context
	cancel   context.CancelFunc
	msgChan  chan kafka.Message
	mu       sync.Mutex
	tracer   trace.Tracer
}

func NewMongoConnect(ctx context.Context, mongoCli *mongo.Client, kafkaReader *kafka.Reader) *MongoConnect {
	db := mongoCli.Database("chatting2")

	coll := mgm.NewCollection(db, mgm.CollName(&mongoAggr{}))
	ctx, cancel := context.WithCancel(ctx)

	reader, _ := otelkafkakonsumer.NewReader(
		kafkaReader,
		// otelkafkakonsumer.WithTracerProvider(tp),
		// otelkafkakonsumer.WithPropagator(propagation.TraceContext{}),
		otelkafkakonsumer.WithAttributes(
			[]attribute.KeyValue{
				{Key: semconv.MessagingDestinationKey, Value: attribute.StringValue(kafkaReader.Config().Topic)},
				semconv.MessagingDestinationKindTopic,
				semconv.MessagingKafkaClientIDKey.String("opentel-manualcommit-cg"),
			},
		),
	)

	c := MongoConnect{
		reader:   reader,
		mongoCli: mongoCli,
		coll:     *coll,
		ctx:      ctx,
		cancel:   cancel,
		msgChan:  make(chan kafka.Message),
		mu:       sync.Mutex{},
		tracer:   otel.Tracer("golang-mongo-connect"),
	}

	go c.run()
	go c.readKafka()

	return &c
}

func (c *MongoConnect) unmarshalKafka(data []byte, v any) error {
	err := json.Unmarshal(data, v)
	if err != nil {
		slog.Error("can not unmarshal kafka message", "data", data, "err", err)
	}
	return err
}

// readKafka read kafkaMessages [kafka.Message] from kafka using batching
// and send them to [MongoConnect.msgChan].
func (c *MongoConnect) readKafka() {
	for {
		var kafkaMsg kafka.Message
		var err error

		select {
		case <-c.ctx.Done():
			return

		default:
			err = c.reader.FetchMessage(context.TODO(), &kafkaMsg)
		}

		if err != nil {
			if err == io.EOF {
				slog.Info("kafka fetch EOF: reader closed")
				return
			}
			slog.Error("get error while reading meassageas", "err", err)
			break
		}

		select {
		case <-c.ctx.Done():
			return

		case c.msgChan <- kafkaMsg:

		}
	}
}

// reads messages [kafka.Message] from [MongoConnect.msgChan]
// in batchSize of 50 items and timout of 100ms.
// then it do transaction.
func (c *MongoConnect) run() {
	messages := make(chan []kafka.Message)

	go func() {
		defer close(messages)

		batchSize := 50
		batchTimout := 100 * time.Millisecond
		msgList := make([]kafka.Message, 0)
		timer := time.NewTimer(batchTimout)
		defer timer.Stop()

		for {
			timer.Reset(batchTimout)

			select {
			case kMsg, ok := <-c.msgChan:
				if !ok {
					return
				}
				msgList = append(msgList, kMsg)

				if len(msgList) == batchSize {
					messages <- msgList
					msgList = make([]kafka.Message, 0)
				}

			case <-timer.C:
				if len(msgList) > 0 {
					messages <- msgList
					msgList = make([]kafka.Message, 0)
				}

			case <-c.ctx.Done():
				return
			}

		}
	}()

	var err error

	for msgList := range messages {
		for i := 0; i < 3; i++ {

			select {
			case <-c.ctx.Done():
				return

			default:
				err = c.prepareAndDoTransaction(context.TODO(), msgList)
				if err != nil {
					slog.Error("transaction failed", "retry", i, "err", err)
				}
			}

			if err == nil {
				break
			}
		}

		if err != nil {
			break
		}
	}
}

// It just prepares.
// It unmarshals messages to [repo.Message] and groups them by their TopicIds.
func (c *MongoConnect) prepareAndDoTransaction(ctx context.Context, msgList []kafka.Message) error {
	propagator := c.reader.TraceConfig.Propagator

	if len(msgList) == 0 {
		return nil
	}

	slog.Info("preparing transaction", "batchLen", len(msgList), "startOffset", msgList[0].Offset)

	ctx, span := c.tracer.Start(ctx, "doTransaction")
	defer span.End()

	topics := make(map[string][]repo.Message)
	for i := range msgList { // group messages by their topicID
		kafkaMsg := msgList[i]

		msg := repo.Message{}
		err := c.unmarshalKafka(kafkaMsg.Value, &msg)
		if err != nil {
			return err
		}

		topics[msg.TopicID] = append(topics[msg.TopicID], msg)

		// Extract tracing info from message
		msgCtx := propagator.Extract(context.Background(), otelkafkakonsumer.NewMessageCarrier(&kafkaMsg))
		trace.SpanFromContext(msgCtx).AddLink(trace.LinkFromContext(ctx))
	}

	err := c.doTransaction(ctx, topics, msgList[len(msgList)-1])

	if err == nil {
		span.SetStatus(codes.Ok, "OK")
		slog.InfoContext(ctx, "tranaction finished succesfuly")
	} else {
		span.SetStatus(codes.Error, "transaction failed")
		span.RecordError(err)
	}

	return err
}

// Executes a transaction to process and store messages in a MongoDB collection,
// and commits a Kafka message upon successful.
//
// An error is returned if the transaction or Kafka message commit fails.
func (c *MongoConnect) doTransaction(
	ctx context.Context,
	topicMap map[string][]repo.Message,
	commitPoint kafka.Message,
	) error {
	err := c.mongoCli.UseSession(ctx, func(sc mongo.SessionContext) (err error) {
		var merged bool
		for topic, msgList := range topicMap {
			agrr := mongoAggr{
				Topic:    topic,
				MinId:    msgList[0].ID,
				MaxId:    msgList[len(msgList)-1].ID,
				Len:      len(msgList),
				Messages: msgList,
			}

			merged, err = c.mergeToLastMessage(sc, &agrr)
			if err != nil {
				break
			}

			if merged {
				_, err = c.coll.ReplaceOne(sc, bson.M{"_id": agrr.ID}, agrr)
			} else {
				_, err = c.coll.InsertOne(sc, agrr)
			}
			if err != nil {
				break
			}
		}

		if err != nil {
			slog.Warn("mongo command failed", "err", err)
			return MongoTransactionErr{fmt.Errorf("mongo command failed: %w", err)}
		}

		err = c.reader.CommitMessages(ctx, commitPoint)
		if err != nil {
			slog.Warn("kafka messages commit failed", "err", err)
			return MongoTransactionErr{fmt.Errorf("kafka messages commit failed: %w", err)}
		}

		return
	})

	if err != nil {
		if _, ok := err.(MongoTransactionErr); !ok {
			slog.Error("posiblity of data loss: transaction failed but commited kafka message", "commitOffset", commitPoint.Offset, "err", err)
		}
	}
	return err
}

// find last item, if len was low append appends old messages to input *agrr*
func (c *MongoConnect) mergeToLastMessage(ctx context.Context, agrr *mongoAggr) (merged bool, err error) {
	getAgrr := mongoAggr{}
	err = c.coll.FirstWithCtx(ctx, bson.M{"topicID": agrr.Topic}, &getAgrr, &options.FindOneOptions{Sort: bson.M{"id": 1}})
	if err != nil {
		if err != mongo.ErrNoDocuments {
			return false, err
		}
	}

	if getAgrr.Len >= 20 || getAgrr.Len == 0 {
		return false, nil
	}

	agrr.Messages = append(getAgrr.Messages, agrr.Messages...)
	agrr.Len = len(agrr.Messages)
	agrr.MinId = getAgrr.MinId
	agrr.ID = getAgrr.ID

	return true, nil
}

func (c *MongoConnect) Close() {
	c.cancel()
}
