package kafkarep

import (
	"context"
	"fmt"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/protocol"
	"github.com/stretchr/testify/assert"
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
	panic("unimplemented")
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
