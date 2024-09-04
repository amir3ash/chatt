package main

import (
	"chat-system/config"
	"chat-system/core/repo"
	kafkarep "chat-system/core/repo/kafkaRep"
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

type Config struct {
	KafkaReader *kafkarep.ReaderConf
	MongoDB     *repo.MongoConf
}

func main() {
	log.SetFlags(log.Lmicroseconds)

	conf := &Config{}
	if err := config.Parse(conf); err != nil {
		panic(err)
	}

	slog.Info("Started")

	otelShutdown, err := setupOTelSDK(context.TODO())
	if err != nil {
		panic(fmt.Errorf("can't setup opentelementry: %w", err))
	}
	defer otelShutdown(context.Background())

	mongoCli := repo.NewInsecureMongoCli(conf.MongoDB)
	defer mongoCli.Disconnect(context.Background())

	kafkaReader := kafkarep.NewInsecureReader(conf.KafkaReader)
	defer kafkaReader.Close()

	mongoKConnect := kafkarep.NewMongoConnect(context.Background(), mongoCli, kafkaReader)
	defer mongoKConnect.Close()

	s := make(chan os.Signal, 1)
	defer close(s)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGINT)

	<-s
}

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
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

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTraceProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	// meterProvider, err := newMeterProvider()
	// if err != nil {
	// 	handleErr(err)
	// 	return
	// }
	// shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	// otel.SetMeterProvider(meterProvider)

	// // Set up logger provider.
	// loggerProvider, err := newLoggerProvider()
	// if err != nil {
	// 	handleErr(err)
	// 	return
	// }
	// shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	// global.SetLoggerProvider(loggerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	// return propagation.NewCompositeTextMapPropagator(
	// 	propagation.TraceContext{},
	// 	propagation.Baggage{},
	// )
	return propagation.TraceContext{}
}

func newTraceProvider() (*trace.TracerProvider, error) {
	ctx := context.Background()
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("chat-mongo-kafka-connect"),
	)

	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint("jaeger-collector.default.svc.cluster.local:4318"),
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
	)
	if err != nil {
		return nil, err
	}

	bsp := trace.NewBatchSpanProcessor(traceExporter, trace.WithBlocking())

	traceProvider := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithSpanProcessor(bsp),
		trace.WithResource(res),
	)
	return traceProvider, nil
}

func newMeterProvider() (*metric.MeterProvider, error) {
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			// Default is 1m. Set to 3s for demonstrative purposes.
			metric.WithInterval(3*time.Second))),
	)
	return meterProvider, nil
}

// func newLoggerProvider() (*otelLog.LoggerProvider, error) {
// 	logExporter, err := stdoutlog.New()
// 	if err != nil {
// 		return nil, err
// 	}

// 	loggerProvider := log.NewLoggerProvider(
// 		log.WithProcessor(log.NewBatchProcessor(logExporter)),
// 	)
// 	return loggerProvider, nil
// }
