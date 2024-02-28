package core

import "context"

func NewService(repo Repository) *svc {
	return &svc{repo: repo}
}

type svc struct {
	repo Repository
	auth AuthZ
}

func (s svc) ListMessages(ctx context.Context, topicID string, p Pagination) ([]Message, error) {
	_, err := s.repo.ListMessages(ctx, topicID, p)
	if err != nil {
		return nil, err
	}
	//TODO implement me
	panic("implement me")
}

func (s svc) SendMessage(ctx context.Context, topicID string, message string) (Message, error) {
	//TODO implement me
	panic("implement me")
}
