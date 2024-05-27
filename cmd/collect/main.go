package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"time"

	slogenv "github.com/cbrewster/slog-env"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"

	"github.com/stormsync/collector"
	"github.com/stormsync/collector/config"
)

var serviceName = semconv.ServiceNameKey.String("main")

func main() {
	logger := slog.New(slogenv.NewHandler(slog.NewTextHandler(os.Stdout, nil)))

	log.Printf("Waiting for connection...")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	vt := os.Getenv("VAULT_TOKEN")

	config, err := config.NewConfig(ctx, vt)
	if err != nil {
		log.Fatal("unable to create new config: ", err)
	}

	interval, err := time.ParseDuration(config.Services.Collector.Interval)
	if err != nil {
		log.Fatal("failed to parse interval: ", err)
	}

	collector, err := collector.NewCollector(ctx, *config, logger)
	if err != nil {
		log.Fatal("failed to create the collector: ", err)
	}

	logger.Info("Starting Collection Service", "collection interval", interval.String())

	collector.Poll(ctx, interval)

}
