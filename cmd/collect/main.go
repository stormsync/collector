package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"time"

	slogenv "github.com/cbrewster/slog-env"

	"github.com/stormsync/collector"
	"github.com/stormsync/collector/config"
)

func main() {
	logger := slog.New(slogenv.NewHandler(slog.NewTextHandler(os.Stdout, nil)))

	log.Printf("Waiting for connection...")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)

	vt := os.Getenv("VAULT_TOKEN")
	var errs error
	collConfig, err := config.NewConfig(ctx, vt)
	if err != nil {
		errs = errors.Join(errs, fmt.Errorf("unable to create new collConfig: %w", err))
	}

	interval, err := time.ParseDuration(collConfig.Services.Collector.Interval)
	if err != nil {
		errs = errors.Join(errs, fmt.Errorf("failed to parse interval: %w", err))
	}

	reportCollector, err := collector.NewCollector(ctx, *collConfig, logger)
	if err != nil {
		errs = errors.Join(errs, fmt.Errorf("failed to create:  %w)", err))
	}

	if errs != nil {
		cancel()
		fmt.Printf("Errors: %s\n", errs)
		os.Exit(1)
	}
	logger.Info("Starting Collection Service", "collection interval", interval.String())
	defer cancel()

	reportCollector.Poll(ctx, interval)
}
