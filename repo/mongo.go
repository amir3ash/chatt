package repo

import (
	"chat-system/core"
	"context"
)

type Repo struct {
}

func (r Repo) ListMessages(ctx context.Context, topicID string, p core.Pagination) ([]core.Message, error) {
	//TODO implement me
	panic("implement me")
}

func (r Repo) SendMsgToTopic(ctx context.Context, topicID string, message string) (core.Message, error) {
	//TODO implement me
	panic("implement me")
}
