package api

import (
	"chat-system/core/messages"
	"context"
	"fmt"
	"time"
)

type MockRepo struct{}

// ListMessages implements core.Repository.
func (m MockRepo) ListMessages(ctx context.Context, topicID string, p messages.Pagination) ([]messages.Message, error) {
	res := make([]messages.Message, 0, p.Limit)
	for i := 0; i < p.Limit; i++ {
		res = append(res, messages.Message{
			SenderId: fmt.Sprint("sender_adfadfadfadfadfadfdfafafadf_", i),
			ID:       fmt.Sprint("id_adfadfadfadfadfadfdfafafadf_", i),
			SentAt:   time.Now(),
			TopicID:  topicID,
			Text:     fmt.Sprint("text_adfadfadfadfadfadfdfafljljdflk jaldkfjalkdjfalkdjflakjdfklkajdfafadf_", i)})
	}

	return res, nil
}

// SendMsgToTopic implements core.Repository.
func (m MockRepo) SendMsgToTopic(ctx context.Context, sender messages.Sender, topicID string, message string) (messages.Message, error) {
	return messages.Message{
		SenderId: sender.ID,
		ID:       fmt.Sprintf("id_%d", time.Now().UnixNano()),
		SentAt:   time.Now(),
		TopicID:  topicID,
		Text:     message,
	}, nil

}

type MockBroker struct{}

func (b MockBroker) SendMessageTo(topicId string, msg *messages.Message) {}

type MockPermissionChecker struct {
}

func (MockPermissionChecker) Check(ctx context.Context, userId, perm, objType, objId string) (bool, error) {
	return true, nil
}

func newMockService() MessageService {
	var svc MessageService = messages.NewService(MockRepo{}, &MockBroker{}, MockPermissionChecker{})
	return svc
}
