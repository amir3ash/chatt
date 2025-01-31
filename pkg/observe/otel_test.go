package observe_test

import (
	"chat-system/pkg/observe"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.opentelemetry.io/otel"
)

func Test(t *testing.T) {
	mock := mock.Mock{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		mock.MethodCalled("handler", r.URL.Path)
		t.Log("INFO: urlPath=", r.URL.Path)
	}))
	defer server.Close()

	mock.On("handler", "/v1/traces").Once()
	mock.On("handler", "/v1/logs").Once()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", server.URL)
	t.Setenv("OTEL_BLRP_SCHEDULE_DELAY", "10") // 10ms for batch log processor
	t.Setenv("OTEL_BSP_SCHEDULE_DELAY", "10")  // 20ms for batch span processor

	observeOpts := observe.Options().
		WithService("service-name", "namespace-test").
		EnableTraceProvider().
		EnableLoggerProvider()

	otelShutdown, err := observe.SetupOTelSDK(context.TODO(), observeOpts)
	if err != nil {
		t.Error(err)
	}
	defer otelShutdown(context.Background())

	_, span := otel.Tracer("t-tracer").Start(context.Background(), "span-name")
	span.End()

	slog.Error("mock custom log")

	time.Sleep(200 * time.Millisecond)

	if !mock.AssertExpectations(t) {
		t.Error("it should send data to http endpoint")
	}
}
