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
	mock.On("handler", "/v1/metrics").Once()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", server.URL)
	t.Setenv("OTEL_BLRP_SCHEDULE_DELAY", "30")    // 30ms for batch log processor
	t.Setenv("OTEL_BSP_SCHEDULE_DELAY", "20")     // 20ms for batch span processor
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "40") // 40ms for metric processor

	observeOpts := observe.Options().
		WithService("service-name", "namespace-test").
		EnableTraceProvider().
		EnableLoggerProvider().
		EnableMeterProvider()

	otelShutdown, err := observe.SetupOTelSDK(context.TODO(), observeOpts)
	if err != nil {
		t.Error(err)
	}
	defer otelShutdown(context.Background())

	_, span := otel.Tracer("t-tracer").Start(context.Background(), "span-name")
	span.End()

	slog.Error("mock custom log")

	counter, _ := otel.Meter("t-meter").Int64Counter("counter")
	counter.Add(context.Background(), 1)

	time.Sleep(100 * time.Millisecond)

	if !mock.AssertExpectations(t) {
		t.Error("it should send data to http endpoint")
	}
}
