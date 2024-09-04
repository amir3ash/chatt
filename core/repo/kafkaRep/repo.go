package kafkarep

import (
	"chat-system/core/messages"
	"chat-system/core/repo"
	"chat-system/ws"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	otelkafkakonsumer "github.com/Trendyol/otel-kafka-konsumer"
	"github.com/kamva/mgm/v3"
	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

type WriterConf struct {
	KafkaHost    string        `env:"KAFKA_HOST"`
	MsgTopic     string        `env:"KAFKA_MSG_TOPIC" default:"chat-messages"`
	BatchTimeout time.Duration `env:"KAFKA_BATCH_TIMEOUT" default:"50ms"`
}

func NewInsecureWriter(conf *WriterConf) *kafka.Writer {
	kafkaWriter := &kafka.Writer{
		Addr:                   kafka.TCP(conf.KafkaHost),
		Topic:                  conf.MsgTopic,
		AllowAutoTopicCreation: true,
		// Balancer:               &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		BatchTimeout: conf.BatchTimeout,
		Compression:  kafka.Snappy,

		// Transport: kafka.DefaultTransport,
	}

	return kafkaWriter
}

type kafkaRepo struct {
	writer *otelkafkakonsumer.Writer
	coll   mgm.Collection
}

func NewKafkaRepo(kafkaWriter *kafka.Writer, db *mongo.Database) *kafkaRepo {
	coll := mgm.NewCollection(db, mgm.CollName(&mongoAggr{}))
	writer, err := otelkafkakonsumer.NewWriter(
		kafkaWriter,
		otelkafkakonsumer.WithAttributes(
			[]attribute.KeyValue{
				semconv.MessagingKafkaClientIDKey.String(kafkaWriter.Stats().ClientID),
			},
		),
	)
	if err != nil {
		panic(err)
	}

	return &kafkaRepo{writer: writer, coll: *coll}
}

func (k kafkaRepo) ListMessages(ctx context.Context, topicID string, pg messages.Pagination) ([]messages.Message, error) {
	p := repo.NewMPaginatin(pg)
	sortStage := bson.M{
		"$sort": bson.M{
			"_id": 1,
		},
	}
	limitStage := bson.M{
		"$limit": p.Limit,
	}

	cur, err := k.coll.Aggregate(ctx, bson.A{
		bson.M{
			"$match": bson.M{
				"minID":   bson.M{"$gt": p.AfterId, "$lt": p.BeforeID},
				"topicID": topicID,
			},
		},
		sortStage,
		limitStage,
		bson.M{
			"$project": bson.M{
				"messages": 1,
				"_id":      0,
			},
		},
		bson.M{
			"$unwind": "$messages",
		},
		bson.M{
			"$replaceRoot": bson.M{
				"newRoot": "$messages",
			},
		},
		bson.M{"$match": bson.M{"_id": bson.M{"$gt": p.AfterId, "$lt": p.BeforeID}}},
		sortStage,
		limitStage,
	},
	)

	if err != nil {
		return nil, err
	}

	defer cur.Close(context.Background())

	res := make([]messages.Message, 0, (pg.Limit))

	for cur.Next(ctx) {
		m := repo.Message{}
		if err := cur.Decode(&m); err != nil {
			return nil, fmt.Errorf("cant decode cursor's element into Message{}, err:%w", err)
		}

		res = append(res, messages.Message{
			SenderId: m.SenderId,
			ID:       m.ID.Hex(),
			TopicID:  m.TopicID,
			SentAt:   m.CreatedAt,
			Text:     m.Text,
		})
	}

	return res, nil
}

func (k kafkaRepo) SendMsgToTopic(ctx context.Context, sender messages.Sender, topicID string, message string) (messages.Message, error) {
	now := time.Now()
	mongoMesg := repo.Message{
		DefaultModel: mgm.DefaultModel{
			IDField:    mgm.IDField{ID: primitive.NewObjectID()},
			DateFields: mgm.DateFields{CreatedAt: now, UpdatedAt: now},
		},
		TopicID:  topicID,
		SenderId: sender.ID,
		Text:     message,
	}

	body, err := json.Marshal(mongoMesg)
	if err != nil {
		return messages.Message{}, err
	}

	kafkaMesg := kafka.Message{
		Key:   []byte(topicID),
		Value: body,
	}

	err = k.writer.WriteMessage(ctx, kafkaMesg)
	if err != nil {
		slog.ErrorContext(ctx, "can not write the message to kafka", "kafkaTopic", k.writer.W.Topic, "err", err)
	}

	return *mongoMesg.ToApiMessage(), err
}

var _ messages.Repository = kafkaRepo{}

type messageChannel struct {
	kafkaReader *kafka.Reader
	ctx         context.Context
}

// read mongo messages from kafka
func NewMessageWatcher(kafkaReader *kafka.Reader) *messageChannel {
	return &messageChannel{kafkaReader: kafkaReader}
}

func (c *messageChannel) WatchMessages() (stream <-chan *repo.ChangeStream, cancel func()) {
	slog.Info("start watching mongodb messages")

	channel := make(chan *repo.ChangeStream)
	go c.watch(channel)

	c.ctx, cancel = context.WithCancel(context.Background())
	stream = channel
	return
}

func (c *messageChannel) watch(channel chan *repo.ChangeStream) {
	defer close(channel)

	for {
		// read msg from kafka and commit it
		kafkaMsg, err := c.kafkaReader.ReadMessage(c.ctx)
		if err != nil {
			if err == io.EOF {
				slog.Error("kafka fetch EOF: reader closed")
				return
			}
			slog.Error("get error while reading meassageas", "err", err)
			break
		}

		msg := repo.Message{}
		err = json.Unmarshal(kafkaMsg.Value, &msg)
		if err != nil {
			slog.Error("can not unmarshal kafka message", "msg", kafkaMsg.Value, "err", err)
			continue
		}
		channel <- &repo.ChangeStream{DocumentKey: msg.ID.Hex(), OperationType: "insert", Msg: msg.ToApiMessage()}
	}
}

var _ = ws.MessageWatcher(&messageChannel{})
