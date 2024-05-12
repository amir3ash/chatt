package core

import (
	"chat-system/authz"
	"context"
	"errors"
)

var ErrNotAuthorized = errors.New("not authorized")

type broker interface {
	SendMessageTo(topicId string, msg *Message)
}

func NewService(repo Repository, broker broker, auth permissionChecker) *svc {
	return &svc{repo: repo, broker: broker, authz: auth}
}

type permissionChecker interface {
	Check(userId, perm, objType, objId string) (bool, error)
}
type svc struct {
	repo   Repository
	authz  permissionChecker
	broker broker
}

func (s svc) ListMessages(ctx context.Context, topicID string, p Pagination) ([]Message, error) {
	can, err := s.authz.Check(authz.UserIdFromCtx(ctx), "read", "topic", topicID)
	if err != nil {
		return nil, err
	}

	if !can {
		return nil, ErrNotAuthorized
	}

	res, err := s.repo.ListMessages(ctx, topicID, p)
	return res, err
}

func (s svc) SendMessage(ctx context.Context, topicID string, message string) (Message, error) {
	userId := authz.UserIdFromCtx(ctx)
	can, err := s.authz.Check(userId, "write", "topic", topicID)
	if err != nil {
		return Message{}, err
	}

	if !can {
		return Message{}, ErrNotAuthorized
	}

	msg, err := s.repo.SendMsgToTopic(ctx, Sender{ID: userId}, topicID, message)
	if err == nil {
		s.broker.SendMessageTo(topicID, &msg)
	}
	return msg, err
}
