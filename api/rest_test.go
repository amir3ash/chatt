package api

//go:generate mockgen -source=handlers.go -destination=mock/service.go

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"chat-system/core"
	mock_core "chat-system/core/mock"

	"github.com/danielgtaylor/huma/v2/humatest"
	"go.uber.org/mock/gomock"
)

var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

func Test_restlistMessages(t *testing.T) {
	ctrl := gomock.NewController(t)

	tests := []struct {
		name             string
		status           int
		topicId          string
		quryPageSize     int
		expectedPageSize int
		expectSvcCalled  bool
	}{
		{"negetive-page-size", http.StatusUnprocessableEntity, "topic-id-1", -1, 0, false},
		{"zero-page-size", http.StatusUnprocessableEntity, "topic-id-2", 0, 0, false},
		{"normal-page-size", http.StatusOK, "topic-id-3", 10, 10, true},
		{"max-page-size-50", http.StatusOK, "topic-id-4", 50, 50, true},
		{"big-page-size", http.StatusUnprocessableEntity, "topic-id-5", 100, 0, false},
		{"without-topic_id", http.StatusNotFound, "", 10, 0, false},
		// {"malformed-topic_id", http.StatusNotFound, "TOPIC%2fmessages#", 10, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mock_core.NewMockService(ctrl)

			_, api := humatest.New(t)
			handler := Handler{
				m,
				"http://test",
			}

			registerEndpoints(api, handler)

			if tt.expectSvcCalled {
				m.
					EXPECT().
					ListMessages(gomock.AssignableToTypeOf(contextType), gomock.Eq(tt.topicId), gomock.Eq(core.Pagination{Limit: tt.expectedPageSize})).
					DoAndReturn(func(context.Context, string, core.Pagination) ([]core.Message, error) {
						return []core.Message{{ID: "id-secret-42"}}, nil
					})
			}

			// Make a GET request
			resp := api.Get(fmt.Sprintf("/topics/%s/messages?limit=%d", tt.topicId, tt.quryPageSize))
			if resp.Code != tt.status {
				t.Fatal("Unexpected status code", resp.Code, "wants", tt.status)
			}

			if resp.Code == 200 && !strings.Contains(resp.Body.String(), "id-secret-42") {
				t.Fatal("unexpected response body, got:", resp.Body.String())
			}

		})
	}
}

func Test_restSendMessage(t *testing.T) {
	ctrl := gomock.NewController(t)

	tests := []struct {
		name            string
		status          int
		topicId         string
		expectSvcCalled bool
		message         string
	}{
		{"normal-req", http.StatusCreated, "topic-id-1", true, "how are you?"},
		{"zero-length-message", http.StatusUnprocessableEntity, "topic-id-1", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mock_core.NewMockService(ctrl)

			_, api := humatest.New(t)
			handler := Handler{
				m,
				"http://test",
			}

			registerEndpoints(api, handler)

			if tt.expectSvcCalled {
				m.
					EXPECT().
					SendMessage(gomock.AssignableToTypeOf(contextType), gomock.Eq(tt.topicId), gomock.Eq(tt.message)).
					DoAndReturn(func(context.Context, string, string) (core.Message, error) {
						return core.Message{ID: "id-secret-42"}, nil
					})
			}

			// Make a GET request
			resp := api.Post(fmt.Sprintf("/topics/%s/messages", tt.topicId), map[string]string{"message": tt.message})
			if resp.Code != tt.status {
				t.Fatal("Unexpected status code", resp.Code, "wants", tt.status)
			}

			if resp.Code == 201 && !strings.Contains(resp.Body.String(), "id-secret-42") {
				t.Fatal("unexpected response body, got:", resp.Body.String())
			}
		})
	}
}

func Test404(t *testing.T) {
	_, api := humatest.New(t)
	handler := Handler{
		nil,
		"",
	}

	registerEndpoints(api, handler)
	resp := api.Put("/some/path")

	if resp.Code != http.StatusNotFound {
		t.Fatal("Unexpected status code", resp.Code)
	}
}
