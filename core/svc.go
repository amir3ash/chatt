package core

import (
	"context"
	"errors"
)

func NewService(repo Repository) *svc {
	return &svc{repo: repo}
}

type svc struct {
	repo  Repository
	authz SimpleAuthz
}

type SimpleAuthz struct {
}

func (s SimpleAuthz) CanUserDo(userId string, verb string, subjectId string) bool {
	return true
}

func (s svc) ListMessages(ctx context.Context, topicID string, p Pagination) ([]Message, error) {
	can := s.authz.CanUserDo("123", "list_messages", topicID)
	if !can {
		return nil, errors.New("not Authorized")
	}

	res, err := s.repo.ListMessages(ctx, topicID, p)
	return res, err
}

func (s svc) SendMessage(ctx context.Context, topicID string, message string) (Message, error) {
	//TODO implement me
	// panic("implement me")

	res, err := s.repo.SendMsgToTopic(ctx, Sender{ID: "123"}, topicID, message)
	return res, err
}
