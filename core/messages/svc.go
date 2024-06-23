package messages

import (
	"chat-system/authz"
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var ErrNotAuthorized = errors.New("not authorized")

// var TopicNotFound = errors.New("topic not found")

func NewService(repo Repository, auth permissionChecker) *svc {
	tracer := otel.GetTracerProvider().Tracer("messages/service")
	
	return &svc{repo: repo, authz: auth, tracer: tracer}
}

type Sender struct{ ID string }

type Message struct {
	SenderId string    `json:"senderId"`
	ID       string    `json:"id"`
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

	tracer trace.Tracer
}

func (s svc) ListMessages(ctx context.Context, topicID string, p Pagination) ([]Message, error) {
	can, err := s.authz.Check(ctx, authz.UserIdFromCtx(ctx), "read", "topic", topicID)
	if err != nil {
		return nil, err
	}

	if !can {
		return nil, ErrNotAuthorized
	}

	res, err := s.repo.ListMessages(ctx, topicID, p)
	return res, err
}

func (s *svc) SendMessage(ctx context.Context, topicID string, message string) (Message, error) {
	ctx, span := s.tracer.Start(ctx, "messageSvc-SendMessage")
	defer span.End()

	userId := authz.UserIdFromCtx(ctx)

	can, err := s.authz.Check(ctx, userId, "write", "topic", topicID)
	if err != nil {
		return Message{}, err
	}

	if !can {
		return Message{}, ErrNotAuthorized
	}

	msg, err := s.repo.SendMsgToTopic(ctx, Sender{ID: userId}, topicID, message)
	return msg, err
}
