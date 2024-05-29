package api

import (
	"chat-system/core/messages"
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

func Benchmark_Handler_listMessages(b *testing.B) {
	svc := newMockService()
	handler := Handler{svc: svc, baseUrl: "http://localhost"}

	limits := []int{1, 10, 20, 50, 100}

	b.ResetTimer()
	b.ReportAllocs()

	for _, l := range limits {
		b.Run(fmt.Sprint("limit_", l), func(b *testing.B) {

			for i := 0; i < b.N; i++ {
				ctx := context.Background()
				input := getMessagesInput{TopicID: "456", Limit: l, BeforeID: "a", AfterID: "000000000000000000000000"}
				output, err := handler.listMessages(ctx, &input)
				if err != nil || output == nil {
					b.Errorf("output=%v   err=%v", output, err)
				}
				if mLen := len(output.Body.Messages); mLen != l {
					b.Errorf("output messages len not equal, expected %d, got %d", l, mLen)
				}
			}
		})
	}

}

type mockService struct {
	err error
}

func (s mockService) ListMessages(ctx context.Context, topicID string, p messages.Pagination) ([]messages.Message, error) {
	return []messages.Message{{ID: "id_test"}}, s.err
}
func (s mockService) SendMessage(ctx context.Context, topicID string, message string) (messages.Message, error) {
	return messages.Message{ID: "id_test"}, s.err
}

func TestHandler_listMessages(t *testing.T) {
	_, api := humatest.New(t)
	mockSvc := &mockService{}
	h := Handler{
		svc:     mockSvc,
		baseUrl: "http://localhost",
	}

	registerEndpoints(api, h)
	
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	type args struct {
		ctx context.Context
		in  *getMessagesInput
	}
	tests := []struct {
		name       string
		args       args
		want       *getMessagesOutput
		svcErr     error
		wantErr    bool
		wantStatsu int
	}{
		{"", args{context.Background(), &getMessagesInput{}}, &getMessagesOutput{}, messages.ErrNotAuthorized, true, 403},
		{"", args{cancelledCtx, &getMessagesInput{}}, &getMessagesOutput{}, messages.ErrNotAuthorized, true, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// resp := api.Get("/topics/TOPIC_ID_TEST/messages")
			// assertStatus(t, resp, 200)
			mockSvc.err = tt.svcErr

			got, err := h.listMessages(tt.args.ctx, tt.args.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("Handler.listMessages() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var errModel huma.StatusError
			errors.As(err, &errModel)
			if errModel.GetStatus() != tt.wantStatsu {
				t.Errorf("listMessages() error = %v, expected %v", err, tt.wantStatsu)
			}
			_ = got
			// if !reflect.DeepEqual(got, tt.want) {
			// 	t.Errorf("Handler.listMessages() = %v, want %v", got, tt.want)
			// }
		})
	}
}

func assertStatus(t *testing.T, resp *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if resp.Code != expected {
		t.Errorf("Unexpected status code. got %d, expected %d", resp.Code, expected)
	}
}
