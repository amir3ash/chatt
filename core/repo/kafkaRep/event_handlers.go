package kafkarep

import (
	"chat-system/core/repo"

	"github.com/kamva/mgm/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mongoMessageHandler interface {
	EventRecieved(MessageEvent)
	Handle(mongo.SessionContext) error
}

type mesgInsertedHandler struct {
	events []MessageInserted
	coll   mgm.Collection
}

// EventRecieved implements messageHandler.
func (m *mesgInsertedHandler) EventRecieved(me MessageEvent) {
	ev := me.(*MessageInserted)
	m.events = append(m.events, *ev)
}

// Handle implements messageHandler.
func (m *mesgInsertedHandler) Handle(sc mongo.SessionContext) error {
	for topicId, events := range groupByTopicId(m.events) {

		msgList := m.extractMessageFromEvents(events)

		agrr := mongoAggr{
			Topic:    topicId,
			MinId:    msgList[0].ID,
			MaxId:    msgList[len(msgList)-1].ID,
			Len:      len(msgList),
			Messages: msgList,
		}

		merged, err := m.mergeToLastMessage(sc, &agrr)
		if err != nil {
			return err
		}

		if merged {
			_, err = m.coll.ReplaceOne(sc, bson.M{"_id": agrr.ID}, agrr)
		} else {
			_, err = m.coll.InsertOne(sc, agrr)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func (*mesgInsertedHandler) extractMessageFromEvents(events []MessageInserted) []repo.Message {
	res := make([]repo.Message, 0, len(events))

	for i := range events {
		res = append(res, events[i].Msg)
	}
	return res
}

// find last item, if len was low append appends old messages to input *agrr*
func (m *mesgInsertedHandler) mergeToLastMessage(sc mongo.SessionContext, agrr *mongoAggr) (merged bool, err error) {
	getAgrr := mongoAggr{}
	err = m.coll.FirstWithCtx(sc, bson.M{"topicID": agrr.Topic}, &getAgrr, &options.FindOneOptions{
		Sort: bson.M{"id": 1},
	})
	if err != nil {
		if err != mongo.ErrNoDocuments {
			return false, err
		}
	}

	if getAgrr.Len >= 20 || getAgrr.Len == 0 {
		return false, nil
	}

	agrr.Messages = append(getAgrr.Messages, agrr.Messages...)
	agrr.Len = len(agrr.Messages)
	agrr.MinId = getAgrr.MinId
	agrr.ID = getAgrr.ID

	return true, nil
}

func groupByTopicId[E MessageEvent](events []E) map[string][]E {
	res := make(map[string][]E, 0)
	for i := range events {
		ev := events[i]
		topicID := ev.TopicID()
		res[topicID] = append(res[topicID], ev)
	}
	return res
}

var _ mongoMessageHandler = &mesgInsertedHandler{}
