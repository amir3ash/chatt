package observe

import (
	"chat-system/version"
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

var instanceId string

func init() {
	instanceId = uuid.NewString()
}

func Options() *options {
	return &options{
		resource: resource.Default(),
	}
}

type options struct {
	err            error
	resource       *resource.Resource
	tracerProvider *trace.TracerProvider
	meterProvider  *metric.MeterProvider
	loggerProvider *log.LoggerProvider
}

func (o *options) handleErr(optionErr error) {
	o.err = errors.Join(o.err, optionErr)
}

// setupOTelSDK bootstraps the OpenTelemetry pipeline globally.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func SetupOTelSDK(ctx context.Context, opts *options) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	if opts.err != nil {
		handleErr(err)
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	if opts.tracerProvider != nil {
		tracerProvider := opts.tracerProvider
		shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
		otel.SetTracerProvider(tracerProvider)
	}

	// Set up meter provider.
	if opts.meterProvider != nil {
		meterProvider := opts.meterProvider
		shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
		otel.SetMeterProvider(meterProvider)
	}

	// Set up logger provider.
	if opts.loggerProvider != nil {
		loggerProvider := opts.loggerProvider
		shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
		global.SetLoggerProvider(loggerProvider)
	}

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	// return propagation.TraceContext{}
}

func (opts *options) EnableTraceProvider() *options {
	ctx := context.Background()

	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
	)
	if err != nil {
		opts.handleErr(err)
		return opts
	}

	if _, ok := os.LookupEnv("OTEL_BSP_SCHEDULE_DELAY"); !ok {
		os.Setenv("OTEL_BSP_SCHEDULE_DELAY", "3000") // ms
	}

	bsp := trace.NewBatchSpanProcessor(traceExporter,
		trace.WithBlocking(),
		trace.WithMaxQueueSize(4096),
	)

	traceProvider := trace.NewTracerProvider(
		trace.WithSampler(trace.TraceIDRatioBased(0.6)),
		trace.WithSpanProcessor(bsp),
		trace.WithResource(opts.resource),
	)

	opts.tracerProvider = traceProvider

	return opts
}

func (opts *options) EnableMeterProvider() *options {
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		opts.handleErr(err)
		return opts
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			// Default is 1m. Set to 3s for demonstrative purposes.
			metric.WithInterval(3*time.Second))),
		metric.WithResource(opts.resource),
	)

	opts.meterProvider = meterProvider
	return opts
}

func (opts *options) EnableLoggerProvider() *options {
	logExporter, err := otlploghttp.New(
		context.Background(),
		otlploghttp.WithCompression(otlploghttp.GzipCompression),
	)
	if err != nil {
		opts.handleErr(err)
		return opts
	}

	if _, ok := os.LookupEnv("OTEL_BLRP_SCHEDULE_DELAY"); !ok {
		os.Setenv("OTEL_BLRP_SCHEDULE_DELAY", "1000") // ms
	}

	blrp := log.NewBatchProcessor(logExporter)

	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(blrp),
		log.WithResource(opts.resource),
	)

	slog.SetDefault(otelslog.NewLogger(""))

	opts.loggerProvider = loggerProvider
	return opts
}

// see https://opentelemetry.io/docs/specs/semconv/resource/
func (opts *options) WithService(serviceName, namespace string) *options {
	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version.Version),
			semconv.ServiceNamespace(namespace),
			semconv.ServiceInstanceID(instanceId),
		),
	)
	if err != nil {
		opts.handleErr(err)
	}

	opts.resource = res
	return opts
}
