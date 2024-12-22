package ws

import (
	"chat-system/core/repo"
)

type MessageWatcher interface {
	// returns change log channel [*repo.ChangeStream]. 
	// cancel func should be called.
	WatchMessages() (stream <-chan *repo.ChangeStream, cancel func())
}

// reads all [*repo.ChangeStream] from a channel
// and calls [roomServer.SendMessageTo] if [repo.ChangeStream.OperationType] is "insert".
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
