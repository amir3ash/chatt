package api

import (
	"chat-system/core"
	"context"
	"github.com/danielgtaylor/huma/v2"
)

type Handler struct {
	svc core.Service
}

func (h *Handler) listMessages(ctx context.Context, in *getMessagesInput) (*getMessagesOutput, error) {
	messages, err := h.svc.ListMessages(ctx, in.TopicID,
		core.Pagination{BeforeID: in.BeforeID, AfterID: in.AfterID, Limit: in.Limit})
	if err != nil {
		return nil, err
	}
	res := &getMessagesOutput{}
	res.Body.Messages = messages
	res.Body.Prev = ""
	res.Body.Next = ""
	// TODO: implement it
	return res, nil
}

func (h *Handler) sendMessage(ctx context.Context, input *sendMessageInput) (*struct{}, error) {
	_, err := h.svc.SendMessage(ctx, input.TopicID, input.Body.Message)
	if err == core.TopicNotFound {
		return nil, huma.Error404NotFound("Topic Not Found", err)
	} else if err != nil {
		return nil, err
	}
	return nil, err
}
