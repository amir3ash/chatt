package ws

import "chat-system/core/messages"

type roomEventType string

const (
	msgSent            roomEventType = "message.sent"
	clientConnected    roomEventType = "client.connected"
	clientDisconnected roomEventType = "client.disconnected"
)

type roomEvent interface {
	EventType() roomEventType
}

type msgEvent struct {
	evType roomEventType
	roomId string
	msg    *messages.Message
}

type clientEvent struct {
	evType roomEventType
	client Client
}

// EventType implements roomEvent.
func (c clientEvent) EventType() roomEventType {
	return c.evType
}

// EventType implements roomEvent.
func (m msgEvent) EventType() roomEventType {
	return m.evType
}

type roomDispatcher struct {
	clientEvSubscribers []func(clientEvent)
	msgEvSubscribers    []func(msgEvent)
}

func NewRoomDispatcher() *roomDispatcher {
	return &roomDispatcher{}
}

func (d roomDispatcher) dispatch(e roomEvent) {
	switch ev := e.(type) {
	case msgEvent:
		for _, f := range d.msgEvSubscribers {
			f(ev)
		}

	case clientEvent:
		for _, f := range d.clientEvSubscribers {
			f(ev)
		}
	}
}

func (d *roomDispatcher) SubscribeOnClientEvents(f func(e clientEvent)) {
	d.clientEvSubscribers = append(d.clientEvSubscribers, f)
}
func (d *roomDispatcher) SubscribeOnMsgEvents(f func(e msgEvent)) {
	d.msgEvSubscribers = append(d.msgEvSubscribers, f)
}

var _ roomEvent = msgEvent{}
var _ roomEvent = clientEvent{}
