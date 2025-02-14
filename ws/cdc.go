package ws

import (
	"chat-system/core/repo"

	"context"

	"go.opentelemetry.io/otel"
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

	traceProvicer := otel.Tracer("MessageCDC")
	for chLog := range stream {
		ctx := context.Background()
		if chLog.Carrier != nil {
			otel.GetTextMapPropagator().Extract(ctx, chLog.Carrier)
		}

		ctx, span := traceProvicer.Start(ctx, "SendMessageTo")

		if chLog.OperationType == "insert" {
			msg := chLog.Msg
			server.SendMessageTo(ctx, msg.TopicID, msg)
		}
		span.End()
	}
}
