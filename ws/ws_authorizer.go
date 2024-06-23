package ws

import (
	"chat-system/authz"
	"cmp"
	"context"
	"fmt"
	"math/rand"
	"slices"
	"strconv"
)


type TestAuthz struct {
	authorizedUserIds []string
	topics            map[string][]string // topic -> userids
}

func NewMockTestAuthz() *TestAuthz {
	lenTopics := 40
	lenUsers := 2001

	mock := TestAuthz{
		authorizedUserIds: make([]string, 2001),
		topics:            make(map[string][]string, lenTopics),
	}

	for i := 0; i < lenUsers; i++ {
		mock.authorizedUserIds[i] = strconv.FormatInt(int64(i), 10)
	}

	for i := 0; i < lenTopics; i++ {
		topicId := fmt.Sprintf("t_%d", i)
		users := []string{}

		for _, userId := range mock.authorizedUserIds {
			if rand.Float32() < 0.25 {
				users = append(users, userId)
			}
		}
		slices.Sort(users)
		users = slices.Compact(users)
		mock.topics[topicId] = users
	}

	// for topic, v := range mock.topics {
	// 	fmt.Println(topic, "->", len(v))
	// }

	for _, userId := range mock.authorizedUserIds {
		numIntrested := 0
		for _, users := range mock.topics {
			_, found := slices.BinarySearch(users, userId)
			if found {
				numIntrested++
			}
		}

		// fmt.Println("user", userId, "->", numIntrested)
	}

	return &mock
}

func (t *TestAuthz) WhoCanWatchTopic(topicId string) ([]string, error) {
	return t.topics[topicId], nil
}

func (t *TestAuthz) TopicsWhichUserCanWatch(userId string, topics []string) (topicIds []string, err error) {
	for i := range topics {
		topic := topics[i]
		users := t.topics[topic]
		_, found := slices.BinarySearch(users, userId)
		if found {
			topicIds = append(topicIds, topic)
		}
	}

	return topicIds, nil
}

// Returns random userid for mocking and testing
func (t TestAuthz) RandomUserId() string {
	return t.authorizedUserIds[rand.Int()%2001]
}

type wsAuthorizer struct {
	authz *authz.Authoriz
}

func NewWSAuthorizer(a *authz.Authoriz) wsAuthorizer {
	return wsAuthorizer{a}
}

func (w wsAuthorizer) WhoCanWatchTopic(topicId string) ([]string, error) {
	return w.authz.WhoHasRel(context.TODO(), "watch", "topic", topicId)
}

func (w wsAuthorizer) TopicsWhichUserCanWatch(userId string, topicsToFilter []string) (topicIds []string, err error) {
	authorizedTopics, err := w.authz.WhichObjsRelateToUser(context.TODO(), userId, "watch", "topic")
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
