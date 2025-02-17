package ws

import (
	"chat-system/authz"
	"cmp"
	"context"
	"slices"
)

// implements [whoCanReadTopic]
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
