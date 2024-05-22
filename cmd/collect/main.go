package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	slogenv "github.com/cbrewster/slog-env"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"

	"github.com/stormsync/collector"
	"github.com/stormsync/collector/config"
)

func main() {
	logger := slog.New(slogenv.NewHandler(slog.NewTextHandler(os.Stdout, nil)))

	environment := os.Getenv("ENVIRONMENT")

	vt := os.Getenv("VAULT_TOKEN")

	config, err := config.NewConfig(environment)
	if err != nil {
		log.Fatal("unable to create new config: ", err)
	}
	config.FetchVaultData(vt)
	var (
		WindURL    = "https://www.spc.noaa.gov/climo/reports/today_wind.csv"
		HailURL    = "https://www.spc.noaa.gov/climo/reports/today_hail.csv"
		TornadoURL = "https://www.spc.noaa.gov/climo/reports/today_torn.csv"
	)

	interval, err := time.ParseDuration(config.Services.Collector.Interval)
	if err != nil {
		log.Fatal("failed to parse interval: ", err)
	}
	// GO_LOG=info;KAFKA_ADDRESS=organic-ray-9236-us1-kafka.upstash.io:9092;KAFKA_PASSWORD=ZGIwNWQwMDYtYjlhMC00NWI3LWFmN2QtMWZlMDVlNDQ0MjEw;KAFKA_USER=b3JnYW5pYy1yYXktOTIzNiQsp1hujJxnWrUY-WO8UK9CyCD7hcKBsa5731Ijvfk;POLLING_INTERVAL_IN_SECONDS=30;REDIS_ADDRESS=gentle-kingfish-45342.upstash.io;REDIS_DB_INT=0;REDIS_PASSWORD=AbEeAAIncDFjMzVlNjAxMTFiYTU0MTFlYWQ1NDk2YmM3NTQ2YmE3YnAxNDUzNDI;REDIS_USER=default;TOPIC=raw-weather-report
	urlMap := map[collector.ReportType]string{
		collector.Wind:    WindURL,
		collector.Hail:    HailURL,
		collector.Tornado: TornadoURL,
	}

	rcc := config.Services.Redis
	rc, err := NewRedisClient(rcc.Host+":"+rcc.Port, rcc.User, rcc.Password, rcc.DB)
	if err != nil {
		log.Fatal("failed to create redis client: ", err)
	}

	// kc := config.Services.Kafka
	kw, err := NewKafkaWriter("kafka-1", "9202", "", "", "")
	if err != nil {
		log.Fatal("unable to create a kafka writer: ", err)
	}

	collector, err := collector.NewCollector(urlMap, rc, kw, logger)
	if err != nil {
		log.Fatal("failed to create the collect: %w", err)
	}

	logger.Info("Starting Collection Service", "collection interval", interval.String())
	logger.Info("collecting source", "wind", WindURL)
	logger.Info("collecting source", "hail", HailURL)
	logger.Info("collecting source", "tornado", TornadoURL)

	ctx := context.Background()
	collector.Poll(ctx, interval)
}

func NewKafkaWriter(host, port, topic, user, pw string) (*kafka.Writer, error) {
	// mechanism, err := scram.Mechanism(scram.SHA256, "", "")
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create scram.Mechanism for auth: %w", err)
	// }
	w := &kafka.Writer{
		Addr: kafka.TCP("kafka-1:9092"),
		// Addr:  kafka.TCP(host + ":" + port),
		Topic: "raw-weather-report",
		// Transport: &kafka.Transport{
		// 	SASL: mechanism,
		// 	TLS:  &tls.Config{},
		// },
	}
	return w, nil
}

func NewRedisClient(address, user, password string, db int) (*redis.Client, error) {
	opt, err := redis.ParseURL(fmt.Sprintf("redis://%s:%s@%s", user, password, address))
	if err != nil {
		return nil, err
	}
	opt.DB = 0
	c := redis.NewClient(opt)

	t := c.Time(context.Background())
	fmt.Println("redis time: ", t.String())
	return c, nil
}
