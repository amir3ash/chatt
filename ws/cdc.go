package ws

import (
	"chat-system/core/repo"
)
type MessageWatcher interface {
	WatchMessages() (stream <-chan *repo.ChangeStream, cancel func()) 
}

func ReadChangeStream(r MessageWatcher, server *roomServer) {
	stream, cancel := r.WatchMessages()
	defer cancel()

	for chLog := range stream {
		if chLog.OperationType == "insert" {
			msg := chLog.Msg
			server.SendMessageTo(msg.TopicID, msg)
		}
	}
}
