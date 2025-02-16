package kafkarep

import (
	"bytes"
	"chat-system/core/messages"
	"chat-system/core/repo"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/kamva/mgm/v3"
	"github.com/segmentio/kafka-go"
)

var _ messages.Repository = kafkaRepo{}

type mockKafkaWriter struct {
	write func(context.Context, kafka.Message) error
}

// Close implements kafkaWriter.
func (m mockKafkaWriter) Close() error {
	panic("unimplemented")
}

// WriteMessage implements kafkaWriter.
func (m mockKafkaWriter) WriteMessage(ctx context.Context, msg kafka.Message) error {
	return m.write(ctx, msg)
}

var _ kafkaWriter = mockKafkaWriter{}

func TestSendMsgToTopic(t *testing.T) {
	mockWriter := mockKafkaWriter{func(ctx context.Context, m kafka.Message) error {
		ev, err := getEventType(&m)
		if err != nil {
			t.Errorf("getEventType returns err: %v", err)
		}

		if ev != EvTypeMessageInserted {
			t.Errorf("SendMsgToTopic sent EventType %v, expected %v", ev, EvTypeMessageInserted)
		}

		if !bytes.Equal(m.Key, []byte("old-friends-topic")) {
			t.Errorf("SendMsgToTopic must set chat's topicID as key, got %s", m.Key)
		}

		return nil
	}}

	repo := kafkaRepo{mockWriter, mgm.Collection{}, "mock-kafka-topic"}

	var nilTime time.Time

	ctx := context.Background()
	tests := []struct {
		name     string
		senderID string
		topicId  string
		msgText  string
		wantErr  bool
	}{
		{"normal", "user-1", "old-friends-topic", "Hello World", false},
		{"empty-sender-id", "", "old-friends-topic", "Hello World", true},
		{"empty-chat-topic", "user-1", "", "Hello World", true},
		{"empty-msg-text", "user-1", "old-friends-topic", "", true},
		{"none-alphanumeric-msg", "user-1", "old-friends-topic", "%%$$%D%F$W@@##--*^\"'-=+\\ \\\\?.\n\n", false},

		// {"forbiden-topic", "user-1", "%%$$%d#\\ \\\\'\"\n\x13\x10\x00%%%", "hello world", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := repo.SendMsgToTopic(ctx, messages.Sender{ID: tt.senderID}, tt.topicId, tt.msgText)
			if (err != nil) != tt.wantErr {
				t.Errorf("kafkaRepo.SendMsgToTopic() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			if msg.SenderId != tt.senderID {
				t.Errorf("got SenderId %s, expected: %s", msg.SenderId, tt.senderID)
			}

			if msg.Text != tt.msgText {
				t.Errorf("got Text %s, expected: %s", msg.Text, tt.msgText)
			}

			if msg.Version != 1 {
				t.Errorf("message Version must be 1, got %d", msg.Version)
			}

			if msg.SentAt == nilTime {
				t.Error("message SentAt is nil")
			}

			if msg.ID == "" {
				t.Error("message ID is empty")
			}
		})
	}
}

func Test_kafkaRepo_marshalEvent(t *testing.T) {
	k := kafkaRepo{}
	inserted := MessageInserted{
		"event-id",
		EvTypeMessageInserted,
		repo.Message{TopicID: "topic"},
	}

	deleted := MessageDeleted{
		"event-id",
		EvTypeMessageDeleted,
		"topic",
		"message-Id",
		5,
		time.Now(),
	}

	insertedBytes, _ := json.Marshal(inserted)
	deltedBytes, _ := json.Marshal(deleted)

	tests := []struct {
		name    string
		ev      Event
		want    []byte
		wantErr bool
	}{
		{"normal-inserted-ev", &inserted, insertedBytes, false},
		{"normal-deleted-ev", &deleted, deltedBytes, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got, err := k.marshalEvent(tt.ev)
			if (err != nil) != tt.wantErr {
				t.Errorf("kafkaRepo.marshalEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("kafkaRepo.marshalEvent() = %v, want %v", got, tt.want)
			}

			unmarshaledEv, err := UnmarshalEvent(tt.ev.EventType(), got)
			if err != nil {
				t.Errorf("UnmarshalEvent returns err: %v", err)
			}

			if !reflect.DeepEqual(unmarshaledEv, tt.ev) {
				t.Errorf("events must be equal when marshaling and unmashaling, got: %+v, expected: %+v", unmarshaledEv, tt.ev)
			}
		})
	}
}
