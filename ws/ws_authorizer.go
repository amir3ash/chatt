package ws

import (
	"chat-system/authz"
	"cmp"
	"math/rand"
	"slices"
	"strconv"
)

var mockAuthorizedUserIds []string

func randomUserId() string {
	return mockAuthorizedUserIds[rand.Int()%1001]
}

type TestAuthz struct{}

func NewMockTestAuthz() TestAuthz {
	mockAuthorizedUserIds = make([]string, 1001)
	for i := range mockAuthorizedUserIds {
		mockAuthorizedUserIds[i] = strconv.FormatInt(int64(i), 10)
	}
	mockAuthorizedUserIds[0] = "343"
	return TestAuthz{}
}

func (t *TestAuthz) WhoCanWatchTopic(topicId string) ([]string, error) {
	return mockAuthorizedUserIds, nil
}

func (t *TestAuthz) TopicsWhichUserCanWatch(userId string, topics []string) (topicIds []string, err error) {
	return topics, nil
}

type wsAuthorizer struct {
	authz *authz.Authoriz
}

func NewWSAuthorizer(a *authz.Authoriz) wsAuthorizer {
	return wsAuthorizer{a}
}

func (w wsAuthorizer) WhoCanWatchTopic(topicId string) ([]string, error) {
	return w.authz.WhoHasRel("watch", "topic", topicId)
}

func (w wsAuthorizer) TopicsWhichUserCanWatch(userId string, topicsToFilter []string) (topicIds []string, err error) {
	authorizedTopics, err := w.authz.WhichObjsRelateToUser(userId, "watch", "topic")
	if err != nil {
		return nil, err
	}

	slices.Sort(authorizedTopics)
	slices.Sort(topicsToFilter)

	for i, j := 0, 0; i < len(authorizedTopics) && j < len(topicsToFilter); {
		a := authorizedTopics[i]
		b := topicsToFilter[j]

		cmp := cmp.Compare(a, b)
		switch cmp {
		case 0:
			topicIds = append(topicIds, a)
			i++
			j++
		case 1: // a is greater than b
			j++
		case -1:
			i++
		}
	}

	return topicIds, nil
}
