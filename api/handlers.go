package api

import (
	"chat-system/core"
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
)

type Handler struct {
	svc     core.Service
	baseUrl string // like https://example.com

}

func (h *Handler) getListMesgLink(in *getMessagesInput) string { // TODO escape user supplied data ( possible open redirect)
	link := fmt.Sprintf("%s/topics/%s/messages?limit=%d", h.baseUrl, in.TopicID, in.Limit)
	if in.AfterID != "" {
		link += "&after_id=" + in.AfterID
	}
	if in.BeforeID != "" {
		link += "&before_id=" + in.BeforeID
	}
	return link
}

func (h *Handler) listMessages(ctx context.Context, in *getMessagesInput) (*getMessagesOutput, error) {
	messages, err := h.svc.ListMessages(ctx, in.TopicID,
		core.Pagination{BeforeID: in.BeforeID, AfterID: in.AfterID, Limit: in.Limit})
	if err != nil {
		return nil, humaErr(err)
	}

	res := &getMessagesOutput{}
	res.Body.Messages = messages

	msgLen := len(messages)
	if msgLen > 0 {
		in.AfterID = ""
		in.BeforeID = messages[0].ID
	}
	res.Body.Prev = h.getListMesgLink(in)

	if msgLen > 0 {
		in.AfterID = messages[msgLen-1].ID
		in.BeforeID = ""
	}
	res.Body.Next = h.getListMesgLink(in)
	return res, nil
}

func (h *Handler) sendMessage(ctx context.Context, input *sendMessageInput) (*ResBody[core.Message], error) {
	msg, err := h.svc.SendMessage(ctx, input.TopicID, input.Body.Message)
	if err != nil {
		return nil, humaErr(err)
	}
	return &ResBody[core.Message]{Body: msg}, err
}

func humaErr(err error) error {
	if err == core.ErrNotAuthorized {
		return huma.Error403Forbidden("not authorized")
	}

	return err
}
