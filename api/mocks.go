package api

import (
	"chat-system/core"
	"context"
	"fmt"
	"time"
)

type MockRepo struct{}

// ListMessages implements core.Repository.
func (m MockRepo) ListMessages(ctx context.Context, topicID string, p core.Pagination) ([]core.Message, error) {
	res := make([]core.Message, 0, p.Limit)
	for i := 0; i < p.Limit; i++ {
		res = append(res, core.Message{
			SenderId: fmt.Sprint("sender_adfadfadfadfadfadfdfafafadf_", i),
			ID:       fmt.Sprint("id_adfadfadfadfadfadfdfafafadf_", i),
			SentAt:   time.Now(),
			TopicID:  topicID,
			Text:     fmt.Sprint("text_adfadfadfadfadfadfdfafljljdflk jaldkfjalkdjfalkdjflakjdfklkajdfafadf_", i)})
	}

	return res, nil
}

// SendMsgToTopic implements core.Repository.
func (m MockRepo) SendMsgToTopic(ctx context.Context, sender core.Sender, topicID string, message string) (core.Message, error) {
	return core.Message{
		SenderId: sender.ID,
		ID:       fmt.Sprintf("id_%d", time.Now().UnixNano()),
		SentAt:   time.Now(),
		TopicID:  topicID,
		Text:     message,
	}, nil

}

type MockBroker struct{}

func (b MockBroker) SendMessageTo(topicId string, msg *core.Message) {}

type MockPermissionChecker struct {
}

func (MockPermissionChecker) Check(userId, perm, objType, objId string) (bool, error) {
	return true, nil
}

func newMockService() core.Service {
	var svc core.Service = core.NewService(MockRepo{}, &MockBroker{}, MockPermissionChecker{})
	return svc
}
