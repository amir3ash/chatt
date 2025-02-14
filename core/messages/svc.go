package messages

import (
	"chat-system/authz"
	"context"
	"errors"
	"time"
)

var ErrEmptyTopicId = errors.New("topicId is empty")

func NewService(repo Repository, auth permissionChecker) *svc {
	return &svc{repo: repo, authz: auth}
}

type Sender struct{ ID string }

type Message struct {
	SenderId string    `json:"senderId"`
	ID       string    `json:"id"`
	Version  uint      `json:"v"`
	TopicID  string    `josn:"topicId"`
	SentAt   time.Time `json:"sentAt"`
	Text     string    `json:"text"`
}

type Pagination struct {
	AfterID  string
	BeforeID string
	Limit    int
}
type svc struct {
	repo  Repository
	authz permissionChecker
}

func (s svc) ListMessages(ctx context.Context, topicID string, p Pagination) ([]Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if topicID == "" {
		return nil, ErrEmptyTopicId
	}

	userId := authz.UserIdFromCtx(ctx)
	can, err := s.authz.Check(ctx, userId, "read", "topic", topicID)
	if err != nil {
		return nil, err
	}

	if !can {
		return nil, ErrNotAuthorized{Subject: userId, ResorceType: "topic", ResorceId: topicID}
	}

	res, err := s.repo.ListMessages(ctx, topicID, p)
	return res, err
}

func (s *svc) SendMessage(ctx context.Context, topicID string, message string) (Message, error) {
	if err := ctx.Err(); err != nil {
		return Message{}, err
	}

	if topicID == "" {
		return Message{}, ErrEmptyTopicId
	}

	userId := authz.UserIdFromCtx(ctx)

	can, err := s.authz.Check(ctx, userId, "write", "topic", topicID)
	if err != nil {
		return Message{}, err
	}

	if !can {
		return Message{}, ErrNotAuthorized{Subject: userId, ResorceType: "topic", ResorceId: topicID}
	}

	msg, err := s.repo.SendMsgToTopic(ctx, Sender{ID: userId}, topicID, message)
	return msg, err
}
