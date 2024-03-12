package core

import (
	"context"
	"errors"
	"time"
)

type Repository interface {
	ListMessages(ctx context.Context, topicID string, p Pagination) ([]Message, error)
	SendMsgToTopic(ctx context.Context,sender Sender, topicID string, message string) (Message, error)
}

type Service interface {
	ListMessages(ctx context.Context, topicID string, p Pagination) ([]Message, error)
	SendMessage(ctx context.Context, topicID string, message string) (Message, error)
}

type AuthZ interface {
}

type Sender struct {ID string}
type Message struct {
	SenderId string    `json:"senderId"`
	ID       string    `json:"id"`
	SentAt   time.Time `json:"sentAt"`
	Text     string    `json:"text"`
}

type Pagination struct {
	AfterID  string
	BeforeID string
	Limit    int
}

var (
	TopicNotFound = errors.New("topic not found")
)
