package ws

import (
	"chat-system/core/repo"
)

func ReadChangeStream(r *repo.Repo, server *roomServer) {
	stream, cancel := r.WatchMessages()
	defer cancel()

	for chLog := range stream {
		if chLog.OperationType == "insert" {
			msg := chLog.Msg
			server.SendMessageTo(msg.TopicID, msg)
		}
	}
}
