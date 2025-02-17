package kafkarep

import (
	"chat-system/core/messages"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"github.com/testcontainers/testcontainers-go/modules/redpanda"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mockKafkaFetch struct {
	fetch func(context.Context, *kafka.Message) error
}

// Close implements KafkaReader.
func (m mockKafkaFetch) Close() error {
	panic("unimplemented")
}

// CommitMessages implements KafkaReader.
func (m mockKafkaFetch) CommitMessages(context.Context, ...kafka.Message) error {
	return nil
}

// FetchMessage implements KafkaReader.
func (m mockKafkaFetch) FetchMessage(ctx context.Context, msg *kafka.Message) error {
	return m.fetch(ctx, msg)
}

var _ KafkaReader = mockKafkaFetch{}

func TestMongoConnect_readKafka(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deadCtx, cancel := context.WithCancel(ctx)
	cancel()

	fetch := func(ctx context.Context, msg *kafka.Message) error {
		msg.Topic = "mock-topic"
		return nil
	}

	newCh := func() chan kafka.Message {
		return make(chan kafka.Message, 1)
	}

	tests := []struct {
		name            string
		fetch           func(ctx context.Context, msg *kafka.Message) error
		ctx             context.Context
		msgChan         chan kafka.Message
		shouldCheckChan bool
		wantErr         bool
	}{
		{"normal", fetch, ctx, newCh(), true, false},
		{"canceled-ctx", fetch, deadCtx, newCh(), false, false},
		{"err-fetch", func(ctx context.Context, msg *kafka.Message) error {
			return fmt.Errorf("mockError")
		}, ctx, newCh(), false, true},

		{"panic in fetch", func(ctx context.Context, msg *kafka.Message) error {
			panic("mock panic")
		}, ctx, newCh(), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			time.Sleep(time.Millisecond) // for t.Errorf race condition
			c := &MongoConnect{
				reader:  mockKafkaFetch{tt.fetch},
				ctx:     tt.ctx,
				msgChan: tt.msgChan,
			}

			go func() {
				err := c.readKafka()
				assert.Equal(t, tt.wantErr, err != nil, "running readKafka wantErr=%v, got err: %v", tt.wantErr, err)
			}()

			if !tt.shouldCheckChan {
				return
			}

			msg := <-tt.msgChan
			if msg.Topic != "mock-topic" {
				t.Error("it should not send nil kafka messages")
			}
		})
	}
}

func startKakfa(t *testing.T, ctx context.Context) (endpoint string) {
	t.Helper()

	redpadaContainer, err := redpanda.Run(ctx, "docker.redpanda.com/redpandadata/redpanda:v24.3.3",
		redpanda.WithAutoCreateTopics(),
	)
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(redpadaContainer); err != nil {
			t.Errorf("failed to terminate container: %s", err)
		}
	})

	if err != nil {
		t.Fatalf("failed to start container: %s", err)
	}

	endpoint, err = redpadaContainer.KafkaSeedBroker(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %s", err)
	}

	return endpoint
}
func StartMongo(t *testing.T, ctx context.Context) (cli *mongo.Client) {
	t.Helper()

	mongodbContainer, err := mongodb.Run(ctx, "mongo:noble")
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(mongodbContainer); err != nil {
			t.Errorf("failed to terminate container: %s", err)
		}
	})

	if err != nil {
		t.Fatalf("failed to start container: %s", err)
	}

	endpoint, err := mongodbContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %s", err)
	}

	mongoCli, err := mongo.Connect(ctx, options.Client().ApplyURI(endpoint))
	if err != nil {
		t.Fatalf("failed to connect to MongoDB: %s", err)
	}

	return mongoCli
}

func Test(t *testing.T) {
	if testing.Short() {
		t.Skip("skip")
	}

	ctx := context.Background()
	kafkaEndpoint := startKakfa(t, ctx)
	mongoCli := StartMongo(t, ctx)

	writerConf := &WriterConf{KafkaHost: kafkaEndpoint, MsgTopic: "topic", BatchTimeout: 50 * time.Millisecond}
	readerConf := &ReaderConf{KafkaHost: kafkaEndpoint, Topic: "topic", MaxBytes: 3000, GroupID: "grp", MaxWait: 300 * time.Millisecond}

	kafkaWriter := NewInsecureWriter(writerConf)
	kafkaReader := NewInsecureReader(readerConf)
	kafkaReader.ReadLag(ctx)

	db := mongoCli.Database("chatting2")
	NewMongoConnect(ctx, db, kafkaReader)

	repo := NewKafkaRepo(kafkaWriter, db)

	sentMsg, err := repo.SendMsgToTopic(ctx, messages.Sender{ID: "sender-id"}, "test-topic", "text")
	if err != nil {
		t.Fatalf("can not send mesg: %v", err)
	}

	_, err = repo.SendMsgToTopic(ctx, messages.Sender{ID: "sender-id"}, "other-topic", "text")
	if err != nil {
		t.Fatalf("can not send mesg to other topic: %v", err)
	}

	time.Sleep(9000 * time.Millisecond)
	slog.Info("list messages")
	messsages, err := repo.ListMessages(ctx, "test-topic", messages.Pagination{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("can not list messages: %v", err)
	}

	if len(messsages) != 1 {
		t.Fatalf("messages' len is %d, expected 1", len(messsages))
	}

	if !cmp.Equal(messsages[0], sentMsg) {
		t.Errorf("messages are not equal, %s", cmp.Diff(messsages[0], sentMsg))
	}
}

func TestDeletedMessageEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("skip mongodb test container")
	}

	ctx := context.Background()
	mongoCli := StartMongo(t, ctx)

	db := mongoCli.Database("chatting2")
	mConnect := NewMongoConnect(ctx, db, &kafka.Reader{})
	// pause kafka reader goroutin
	mConnect.reader = mockKafkaFetch{func(ctx context.Context, m *kafka.Message) error {
		time.Sleep(time.Hour)
		return nil
	}}

	kafkaRepo := NewKafkaRepo(&kafka.Writer{}, db)
	// send kafkaRepo kafka.Message to MongoConnect
	kafkaRepo.writer = mockKafkaWriter{func(_ context.Context, m kafka.Message) error {
		mConnect.msgChan <- m
		return nil
	}}

	sentMsg, err := kafkaRepo.SendMsgToTopic(ctx, messages.Sender{ID: "sender-id"}, "test-topic", "text")
	if err != nil {
		t.Fatalf("can not send mesg: %v", err)
	}

	_, err = kafkaRepo.SendMsgToTopic(ctx, messages.Sender{ID: "sender-id"}, "test-topic", "other message")
	if err != nil {
		t.Fatalf("can not send 2nd mesg: %v", err)
	}

	time.Sleep(600 * time.Millisecond)
	slog.Info("list messages")
	messsages, err := kafkaRepo.ListMessages(ctx, "test-topic", messages.Pagination{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("can not list messages: %v", err)
	}

	if len(messsages) != 2 {
		t.Fatalf("messages' len is %d, expected 2", len(messsages))
	}

	err = kafkaRepo.DeleteMessage(ctx, &sentMsg)
	if err != nil {
		t.Fatalf("can not delete message: %v", err)
	}

	time.Sleep(600 * time.Millisecond)

	messsages, err = kafkaRepo.ListMessages(ctx, "test-topic", messages.Pagination{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("can not list messages: %v", err)
	}

	if len(messsages) != 1 {
		t.Errorf("message should be deleted, msgs=%v", messsages)
	}

	err = kafkaRepo.DeleteMessage(ctx, &sentMsg)
	if err != nil {
		t.Fatalf("delete message should be idempotent: %v", err)
	}
}

func Test_getEventType(t *testing.T) {
	tests := []struct {
		name       string
		msg        *kafka.Message
		wantEvType EventType
		wantErr    bool
	}{
		{"normal", &kafka.Message{Headers: []protocol.Header{{
			Key: "eventType", Value: []byte(EvTypeMessageInserted)}},
		}, EvTypeMessageInserted, false},

		{"without-headers", &kafka.Message{}, "", true},
		{"nil-message", nil, "", true},

		{"without-eventType", &kafka.Message{Headers: []protocol.Header{{
			Key: "something", Value: []byte(EvTypeMessageInserted)}},
		}, "", true},

		{"should-return-first", &kafka.Message{Headers: []protocol.Header{
			{Key: "eventType", Value: []byte(EvTypeMessageInserted)},
			{Key: "eventType", Value: []byte(EvTypeMessageDeleted)},
		}}, EvTypeMessageInserted, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvType, err := getEventType(tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("getEventType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotEvType != tt.wantEvType {
				t.Errorf("getEventType() = %v, want %v", gotEvType, tt.wantEvType)
			}
		})
	}
}

func newDummyKafkaMessage(event MessageEvent) kafka.Message {
	data, _ := json.Marshal(event)

	return kafka.Message{
		Headers: []kafka.Header{{Key: "eventType", Value: []byte(event.EventType())}},
		Value:   data,
	}
}
