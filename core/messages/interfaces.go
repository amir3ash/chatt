package messages

import (
	"context"
)

//go:generate mockgen -typed -source=interfaces.go -destination=mock/interfaces.go

type permissionChecker interface {
	Check(ctx context.Context, userId, perm, objType, objId string) (bool, error)
}

type Repository interface {
	ListMessages(ctx context.Context, topicID string, p Pagination) ([]Message, error)
	SendMsgToTopic(ctx context.Context, sender Sender, topicID string, message string) (Message, error)
}




