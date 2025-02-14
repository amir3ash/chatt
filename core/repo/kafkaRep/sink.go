package kafkarep

import (
	"chat-system/core/repo"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	otelkafkakonsumer "github.com/Trendyol/otel-kafka-konsumer"
	"github.com/kamva/mgm/v3"
	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var ErrEventTypeHeaderNotFound = errors.New("eventType header not found in kafka.Message")

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

// interface for [kafka.Reader]
type KafkaReader interface {
	// see [kafka.Reader.FetchMessage]
	FetchMessage(context.Context, *kafka.Message) error

	// see [kafka.Reader.CommitMessages]
	CommitMessages(context.Context, ...kafka.Message) error

	Close() error
}

// MongoConnect is responsible for reading messages from Kafka
// and write messages to MongoDB.
type MongoConnect struct {
	reader   KafkaReader
	mongoCli *mongo.Client
	coll     mgm.Collection
	ctx      context.Context
	cancel   context.CancelFunc
	msgChan  chan kafka.Message
	tracer   trace.Tracer
	handlers map[EventType]mongoMessageHandler
}

func NewMongoConnect(ctx context.Context, mongoDB *mongo.Database, kafkaReader *kafka.Reader) *MongoConnect {
	coll := mgm.NewCollection(mongoDB, mgm.CollName(&mongoAggr{}))
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
		mongoCli: mongoDB.Client(),
		coll:     *coll,
		ctx:      ctx,
		cancel:   cancel,
		msgChan:  make(chan kafka.Message),
		tracer:   otel.Tracer("golang-mongo-connect"),
		handlers: make(map[EventType]mongoMessageHandler),
	}

	go c.run()

	go func() {
		for {
			err := c.readKafka()
			if err == nil {
				break
			}

			// TODO: handle failure
			time.Sleep(100 * time.Millisecond)
		}
	}()

	return &c
}

// readKafka fetches kafkaMessages [kafka.Message] from kafka
// and send them to [MongoConnect.msgChan].
//
// returns nil if MongoConnect.ctx cancelled.
func (c *MongoConnect) readKafka() (err error) {
	defer func() {
		r := recover()
		if r != nil {
			slog.Error("panic in MongoConnect.readKafka", "err", r)
			err = fmt.Errorf("panic: %v", r)
		}

		if err == nil {
			close(c.msgChan)
		}
	}()

	for {
		var kafkaMsg kafka.Message

		select {
		case <-c.ctx.Done():
			return nil

		default:
			err = c.reader.FetchMessage(context.TODO(), &kafkaMsg)
		}

		if err != nil {
			if err == io.EOF {
				slog.Info("kafka reader closed")
				return err
			}
			slog.Error("fetching kafka messages failed", "err", err)
			return err
		}

		select {
		case <-c.ctx.Done():
			return nil

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

// returns a mongodb transaction handler for specific [EventType].
func (c *MongoConnect) getHandler(ev MessageEvent) mongoMessageHandler {
	handler, ok := c.handlers[ev.EventType()]
	if ok {
		return handler
	}

	switch ev.EventType() {
	case EvTypeMessageInserted:
		handler = &mesgInsertedHandler{coll: c.coll}
	}

	c.handlers[ev.EventType()] = handler
	return handler
}

func (c *MongoConnect) prepareAndDoTransaction(ctx context.Context, msgList []kafka.Message) error {
	if len(msgList) == 0 {
		return nil
	}
	propagator := propagation.NewCompositeTextMapPropagator()

	slog.Info("preparing transaction", "batchLen", len(msgList), "startOffset", msgList[0].Offset)

	ctx, span := c.tracer.Start(ctx, "doTransaction")
	defer span.End()

	for i := range msgList {
		kafkaMsg := msgList[i]
		eventType, err := getEventType(&kafkaMsg)
		if err != nil {
			slog.Error("can not get eventType",
				slog.String("topic", kafkaMsg.Topic),
				slog.Int("partition", kafkaMsg.Partition),
				slog.Int64("offset", kafkaMsg.Offset),
				"err", err)

			return err
		}

		event, err := UnmarshalEvent(eventType, kafkaMsg.Value)
		if err != nil {
			slog.Error("can not unmarshal event", "err", err)
			return err
		}

		msgEvent := event.(MessageEvent)
		c.getHandler(msgEvent).EventRecieved(msgEvent)

		// Extract tracing info from message
		msgCtx := propagator.Extract(context.Background(), otelkafkakonsumer.NewMessageCarrier(&kafkaMsg))
		trace.SpanFromContext(msgCtx).AddLink(trace.LinkFromContext(ctx))
	}

	err := c.doTransaction(ctx, msgList[len(msgList)-1])

	if err == nil {
		span.SetStatus(codes.Ok, "OK")
		slog.InfoContext(ctx, "transaction finished succesfuly")
	} else {
		span.SetStatus(codes.Error, "transaction failed")
		span.RecordError(err)
	}

	clear(c.handlers)
	return err
}

// returns [EventType] from kafka header "eventType".
// Returns error if header not found or type not found.
func getEventType(m *kafka.Message) (evType EventType, err error) {
	if m == nil {
		return "", errors.New("kafkaMessage is nil")
	}

	for _, h := range m.Headers {
		if h.Key == "eventType" {
			evType, err = ValidateEventType(h.Value)
			if err != nil {
				return "", err
			}
			break
		}
	}

	if evType == "" {
		return "", ErrEventTypeHeaderNotFound
	}

	return evType, nil
}

// Executes a transaction to process and store messages in a MongoDB collection,
// and commits a Kafka message upon successful.
//
// An error is returned if the transaction or Kafka message commit fails.
func (c *MongoConnect) doTransaction(
	ctx context.Context,
	commitPoint kafka.Message,
) error {
	err := c.mongoCli.UseSession(ctx, func(sc mongo.SessionContext) (err error) {
		for _, handler := range c.handlers {
			err = handler.Handle(sc)
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

func (c *MongoConnect) Close() {
	c.cancel()
}
