package kafkarep

import (
	"chat-system/core/messages"
	"chat-system/core/repo"
	"chat-system/ws"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	otelkafkakonsumer "github.com/Trendyol/otel-kafka-konsumer"
	"github.com/kamva/mgm/v3"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/protocol"
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
		RequiredAcks:           kafka.RequireOne,
		BatchTimeout:           conf.BatchTimeout,
		Compression:            kafka.Snappy,
	}

	return kafkaWriter
}

type kafkaWriter interface {
	WriteMessage(context.Context, kafka.Message) error
	Close() error
}

type kafkaRepo struct {
	writer        kafkaWriter
	coll          mgm.Collection
	messagesTopic string
}

func NewKafkaRepo(kafkaWriter *kafka.Writer, db *mongo.Database) *kafkaRepo {
	coll := mgm.NewCollection(db, mgm.CollName(&mongoAggr{}))
	return &kafkaRepo{
		writer:        createWriter(kafkaWriter),
		coll:          *coll,
		messagesTopic: kafkaWriter.Topic,
	}
}

func createWriter(kafkaWriter *kafka.Writer) *otelkafkakonsumer.Writer {
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
	return writer
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
		bson.M{"$match": bson.M{
			"_id":     bson.M{"$gt": p.AfterId, "$lt": p.BeforeID},
			"deleted": false,
		}},
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
			Version:  m.Version,
		})
	}

	return res, nil
}

var ErrEmptyArgs = errors.New("empty args")

// creates new [MessageInserted] event and send it to Kafka.
func (k kafkaRepo) SendMsgToTopic(ctx context.Context, sender messages.Sender, topicID string, message string) (messages.Message, error) {
	if err := ctx.Err(); err != nil {
		return messages.Message{}, err
	}

	if sender.ID == "" || message == "" || topicID == "" {
		return messages.Message{}, ErrEmptyArgs
	}

	now := time.Now()
	mongoMesg := repo.Message{
		DefaultModel: mgm.DefaultModel{
			IDField:    mgm.IDField{ID: primitive.NewObjectID()},
			DateFields: mgm.DateFields{CreatedAt: now, UpdatedAt: now},
		},
		TopicID:  topicID,
		SenderId: sender.ID,
		Text:     message,
		Version:  1,
	}

	event := MessageInserted{
		EventId: NewEventID(),
		EvType:  EvTypeMessageInserted,
		Msg:     mongoMesg,
	}

	body, err := k.marshalEvent(&event)
	if err != nil {
		return messages.Message{}, err
	}

	kafkaMesg := kafka.Message{
		Key:   []byte(topicID),
		Value: body,
		Headers: []kafka.Header{
			protocol.Header{
				Key:   "eventType",
				Value: []byte(event.EvType),
			},
		},
	}

	err = k.writer.WriteMessage(ctx, kafkaMesg)
	if err != nil {
		slog.ErrorContext(ctx, "can not write the message to kafka", "kafkaTopic", k.messagesTopic, "err", err)
	}

	return *mongoMesg.ToApiMessage(), err
}

// DeleteMessage implements messages.Repository.
func (k kafkaRepo) DeleteMessage(ctx context.Context, msg *messages.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if msg == nil {
		return errors.New("message is nil")
	}

	event := MessageDeleted{
		EventId:        NewEventID(),
		EvType:         EvTypeMessageDeleted,
		TopicId:        msg.TopicID,
		MessageId:      msg.ID,
		MessageVersion: msg.Version,
	}

	body, err := k.marshalEvent(&event)
	if err != nil {
		return err
	}

	kafkaMesg := kafka.Message{
		Key:   []byte(msg.TopicID),
		Value: body,
		Headers: []kafka.Header{
			protocol.Header{
				Key:   "eventType",
				Value: []byte(event.EvType),
			},
		},
	}

	err = k.writer.WriteMessage(ctx, kafkaMesg)
	if err != nil {
		slog.ErrorContext(ctx, "can not write the message to kafka", "kafkaTopic", k.messagesTopic, "err", err)
	}

	return err
}

func (kafkaRepo) marshalEvent(m Event) ([]byte, error) {
	bytes, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

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

		eventType, err := getEventType(&kafkaMsg)
		if err != nil {
			slog.Error("can not get EventType", "err", err)
			continue
		}

		event, err := UnmarshalEvent(eventType, kafkaMsg.Value)
		if err != nil {
			slog.Error("can not unmarshal event", "err", err)
			continue
		}

		msg := event.(MessageInserted).Msg
		carrier := otelkafkakonsumer.NewMessageCarrier(&kafkaMsg)

		channel <- &repo.ChangeStream{
			DocumentKey:   msg.ID.Hex(),
			OperationType: "insert",
			Msg:           msg.ToApiMessage(),
			Carrier:       carrier,
		}
	}
}

var _ = ws.MessageWatcher(&messageChannel{})
