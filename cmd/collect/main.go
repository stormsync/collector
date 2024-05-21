package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"strconv"
	"time"

	slogenv "github.com/cbrewster/slog-env"

	"github.com/stormsync/collector"
)

func main() {
	logger := slog.New(slogenv.NewHandler(slog.NewTextHandler(os.Stdout, nil)))

	var (
		WindURL    = "https://www.spc.noaa.gov/climo/reports/today_wind.csv"
		HailURL    = "https://www.spc.noaa.gov/climo/reports/today_hail.csv"
		TornadoURL = "https://www.spc.noaa.gov/climo/reports/today_torn.csv"
	)

	var envFail = false
	address := os.Getenv("KAFKA_ADDRESS")
	if address == "" {
		envFail = true
		logger.Error("ENV variable required", "Name", "KAFKA_ADDRESS")
	}

	user := os.Getenv("KAFKA_USER")
	if user == "" {
		envFail = true
		logger.Error("ENV variable required", "Name", "KAFKA_USER")
	}
	pw := os.Getenv("KAFKA_PASSWORD")
	if pw == "" {
		envFail = true
		logger.Error("ENV variable required", "Name", "KAFKA_PASSWORD")
	}
	topic := os.Getenv("TOPIC")
	if topic == "" {
		envFail = true
		logger.Error("ENV variable required", "Name", "TOPIC")
	}

	intervalDuration, err := time.ParseDuration(os.Getenv("POLLING_INTERVAL_IN_SECONDS") + "s")
	if err != nil {
		envFail = true
		logger.Error("ENV variable required", "Name", "POLLING_INTERVAL_IN_SECONDS")
	}

	redisAddress := os.Getenv("REDIS_ADDRESS")
	if redisAddress == "" {
		envFail = true
		logger.Error("ENV variable required", "Name", "REDIS_ADDRESS")

	}
	redisUser := os.Getenv("REDIS_USER")
	if redisUser == "" {
		envFail = true
		logger.Error("ENV variable required", "Name", "REDIS_USER")
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")
	if redisPassword == "" {
		envFail = true
		logger.Error("ENV variable required", "Name", "REDIS_PASSWORD")
	}

	if envFail {
		os.Exit(1)
	}
	redisDB := os.Getenv("REDIS_DB_INT")
	dbInt, err := strconv.Atoi(redisDB)
	if err != nil {
		dbInt = 0
	}

	urlMap := map[collector.ReportType]string{
		collector.Wind:    WindURL,
		collector.Hail:    HailURL,
		collector.Tornado: TornadoURL,
	}

	logger.Info("Starting Collection Service", "collection interval", intervalDuration.String())
	logger.Info("collecting source", "wind", WindURL)
	logger.Info("collecting source", "hail", HailURL)
	logger.Info("collecting source", "tornado", TornadoURL)
	rc, err := collector.NewRedisClient(redisAddress, redisUser, redisPassword, dbInt)
	if err != nil {
		log.Fatal("failed to create redis client: ", err)
	}

	collector, err := collector.NewCollector(address, topic, user, pw, urlMap, rc, logger)
	if err != nil {
		log.Fatal("failed to create the collect: %w", err)
	}

	ctx := context.Background()

	collector.Poll(ctx, intervalDuration)
}
