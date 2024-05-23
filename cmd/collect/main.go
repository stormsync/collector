package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	slogenv "github.com/cbrewster/slog-env"
	jaegerPropagator "go.opentelemetry.io/contrib/propagators/jaeger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"

	"github.com/stormsync/collector"
	"github.com/stormsync/collector/internal/config"
)

func main() {
	// Create resource.

	// const op = "api.banners.CreateBanner"
	logger := slog.New(slogenv.NewHandler(slog.NewTextHandler(os.Stdout, nil)))
	otel.SetTextMapPropagator(jaegerPropagator.Jaeger{})
	ctx := context.Background()
	traceProvider, err := startTracer()
	if err != nil {
		log.Fatal("Unable to initiate tracer: ", err)
	}
	defer func() {
		if err := traceProvider.Shutdown(context.Background()); err != nil {
			log.Fatalf("traceprovider: %v", err)
		}
	}()

	tracer := traceProvider.Tracer("collector")
	ctx, span := tracer.Start(ctx, "main")
	defer span.End()
	startTracer()
	vt := os.Getenv("VAULT_TOKEN")

	config, err := config.NewConfig(ctx, vt)
	if err != nil {
		log.Fatal("unable to create new config: ", err)
	}

	interval, err := time.ParseDuration(config.Services.Collector.Interval)
	if err != nil {
		log.Fatal("failed to parse interval: ", err)
	}

	collector, err := collector.NewCollector(ctx, *config, tracer, logger)
	if err != nil {
		log.Fatal("failed to create the collect: ", err)
	}

	logger.Info("Starting Collection Service", "collection interval", interval.String())

	ctx = context.WithoutCancel(ctx)
	collector.Poll(ctx, interval)
}

func startTracer() (*trace.TracerProvider, error) {
	headers := map[string]string{
		"content-type": "application/json",
	}

	exporter, err := otlptrace.New(
		context.Background(),
		otlptracehttp.NewClient(
			otlptracehttp.WithEndpoint("localhost:4318"),
			otlptracehttp.WithHeaders(headers),
			otlptracehttp.WithInsecure(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create new tracing exporter: %w", err)
	}

	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(
			exporter,
			trace.WithMaxExportBatchSize(trace.DefaultMaxExportBatchSize),
			trace.WithBatchTimeout(trace.DefaultScheduleDelay*time.Millisecond),
			trace.WithMaxExportBatchSize(trace.DefaultMaxExportBatchSize),
		),
		trace.WithResource(
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String("collector"),
			),
		),
	)

	otel.SetTracerProvider(tracerProvider)

	return tracerProvider, nil

}
