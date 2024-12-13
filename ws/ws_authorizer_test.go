package ws

import (
	"fmt"
	"math/rand/v2"
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
