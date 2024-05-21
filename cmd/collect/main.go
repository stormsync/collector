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

	address := os.Getenv("KAFKA_ADDRESS")
	if address == "" {
		log.Fatal("address is required.  Use env var ADDRESS")
	}

	user := os.Getenv("KAFKA_USER")
	if user == "" {
		log.Fatal("kafka user is required.  Use env var KAFKA_USER")
	}
	pw := os.Getenv("KAFKA_PASSWORD")
	if pw == "" {
		log.Fatal("kafka password is required.  Use env var KAFKA_PASSWORD")
	}
	topic := os.Getenv("TOPIC")
	if topic == "" {
		log.Fatal("topic is required.  Use env var TOPIC")
	}

	intervalStr := os.Getenv("POLLING_INTERVAL_IN_SECONDS")
	if intervalStr == "" {
		log.Fatal("polling interval is required.  Use env var POLLING_INTERVAL_IN_SECONDS")
	}
	_, err := strconv.Atoi(intervalStr)
	if err != nil {
		log.Fatal("interval needs to be an integer in quotes")
	}
	logger.Debug("polling interval", "in seconds", intervalStr)

	intervalDuration, err := time.ParseDuration(intervalStr + "s")
	if err != nil {
		log.Fatal("only integers, representing seconds, are allowed for interval")
	}

	redisAddress := os.Getenv("REDIS_ADDRESS")
	if redisAddress == "" {
		log.Fatal("redis addres required.  Use env var REDIS_ADDRESS")

	}
	redisUser := os.Getenv("REDIS_USER")
	if redisUser == "" {
		log.Fatal("redis user required.  Use env var REDIS_USER")
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")
	if redisPassword == "" {
		log.Fatal("redis password required.  Use env var REDIS_PASSWORD")
	}

	redisDB := os.Getenv("REDIS_DB_INT")
	dbInt, err := strconv.Atoi(redisDB)
	if err != nil {
		logger.Info("db not provided, defaulting to 0")
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
