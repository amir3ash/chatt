package kafkarep

import (
	"chat-system/core/repo"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type EventType string

const (
	EvTypeMessageInserted EventType = "message.inserted.v1"
	EvTypeMessageDeleted  EventType = "message.deleted.v1"
)

func ValidateEventType(t []byte) (EventType, error) {
	if len(t) < 3 {
		return "", fmt.Errorf("invalid EventType %s", t)
	}

	s := EventType(t)
	switch s {
	case EvTypeMessageInserted, EvTypeMessageDeleted:
		return s, nil
	}

	return "", fmt.Errorf("invalid EventType %s", t)
}

type EventID string

func NewEventID() EventID {
	return EventID(uuid.NewString())
}

type Event interface {
	EventID() EventID
	EventType() EventType
}

type MessageEvent interface {
	Event
	TopicID() string
}

type MessageInserted struct {
	EventId EventID      `json:"event_id,omitempty"`
	EvType  EventType    `json:"event_type,omitempty"`
	Msg     repo.Message `json:"msg,omitempty"`
}

// TopicID implements MessageEvent.
func (e MessageInserted) TopicID() string {
	return e.Msg.TopicID
}

type MessageDeleted struct {
	EventId        EventID   `json:"event_id"`
	EvType         EventType `json:"event_type"`
	TopicId        string    `json:"topic_id"`
	MessageId      string    `json:"message_id"`
	MessageVersion uint      `json:"message_version,omitempty"`
	DeletedAt      time.Time `json:"deleted_at"`
}

// TopicID implements MessageEvent.
func (e MessageDeleted) TopicID() string {
	return e.TopicId
}

type TextEdited struct {
	EventId        EventID   `json:"event_id,omitempty"`
	EvType         EventType `json:"event_type,omitempty"`
	MessageId      string    `json:"message_id,omitempty"`
	MessageVersion uint      `json:"message_version,omitempty"`
	NewText        string    `json:"new_text,omitempty"`
}

func (e MessageInserted) EventID() EventID {
	return e.EventId
}
func (e MessageInserted) EventType() EventType {
	return e.EvType
}

func (e MessageDeleted) EventID() EventID {
	return e.EventId
}
func (e MessageDeleted) EventType() EventType {
	return e.EvType
}

func (e TextEdited) EventID() EventID {
	return e.EventId
}
func (e TextEdited) EventType() EventType {
	return e.EvType
}

func UnmarshalEvent(t EventType, v []byte) (ev Event, err error) {
	switch t {
	case EvTypeMessageInserted:
		ev = &MessageInserted{}

	case EvTypeMessageDeleted:
		ev = &MessageDeleted{}

	default:
		return nil, fmt.Errorf("eventType %s not found", t)
	}

	err = json.Unmarshal(v, ev)
	return ev, err
}

var _ Event = MessageInserted{}
var _ Event = MessageDeleted{}

var _ MessageEvent = MessageInserted{}
var _ MessageEvent = MessageDeleted{}
