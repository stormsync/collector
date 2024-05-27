package collector

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"

	"github.com/stormsync/collector/config"
)

//go:generate stringer -type=ReportType
type ReportType int

const (
	Hail ReportType = iota
	Wind
	Tornado
)

type CollectorConfig struct {
	Redis struct {
		User     string
		Password string
		Host     string
		Port     string
		DBID     int
	}
	Kafka struct {
		User     string
		Password string
		Host     string
		Port     string
		Topic    string
	}
	CollectionURLs map[ReportType]string
}

type Collector struct {
	topic          string
	collectionURLs []config.CollectionUrls
	producer       *kafka.Writer
	lineHashes     map[string]struct{}
	hasher         hash.Hash
	logger         *slog.Logger

	redis *redis.Client
}

func FromString(reportType string) (ReportType, error) {
	switch reportType {
	case Hail.String():
		return Hail, nil
	case Wind.String():
		return Wind, nil
	case Tornado.String():
		return Tornado, nil
	default:
		return 0, errors.New("unknown report type")
	}
}

// NewCollector generates a new collector that has a kafka writer configured and ready to write.
func NewCollector(ctx context.Context, c config.Config, l *slog.Logger) (*Collector, error) {
	if l == nil {
		return nil, errors.New("logger cannot be nil")
	}

	if c.Services.Collector.CollectionUrls == nil {
		return nil, errors.New("collection urls cannot be nil")
	}

	if c.Services.Kafka.Topic == "" {
		return nil, errors.New("kafka topic is required")
	}
	kw, err := newKafkaWriter(c.Services.Kafka.Host, c.Services.Kafka.Topic, c.Services.Kafka.User, c.Services.Kafka.Password, c.Services.Kafka.Port)
	if err != nil {
		return nil, fmt.Errorf("failed to create a kafka writer: %w", err)
	}

	rc := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", c.Services.Redis.Host, &c.Services.Redis.Port),
		Username: c.Services.Redis.User,
		Password: c.Services.Redis.Password,
		DB:       c.Services.Redis.Db,
	})

	return &Collector{
		topic:          kw.Topic,
		collectionURLs: c.Services.Collector.CollectionUrls,
		producer:       kw,
		logger:         l,

		redis: rc,
	}, nil
}

func newKafkaWriter(host, topic, user, pw string, port int) (*kafka.Writer, error) {
	w := &kafka.Writer{
		Addr:  kafka.TCP(fmt.Sprintf("%s:%d", host, port)),
		Topic: topic,
	}
	if user != "" && pw != "" {
		mechanism, err := scram.Mechanism(scram.SHA256, "", "")
		if err != nil {
			return nil, fmt.Errorf("failed to create scram.Mechanism for auth: %w", err)
		}
		w.Transport = &kafka.Transport{
			SASL: mechanism,
			TLS:  &tls.Config{},
		}
	}
	return w, nil
}

// collect does the work of making the get request and checking the resp for validity.
func (c *Collector) collect(ctx context.Context, url string, logger *slog.Logger) ([]byte, error) {

	var rb []byte

	resp, err := http.Get(url)
	if err != nil {

		return rb, fmt.Errorf("error making get request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {

		logger.Debug("response status", "status code", resp.StatusCode, "status", resp.Status)
		return rb, fmt.Errorf("non-200 status code returned: %s", resp.Status)
	}

	defer resp.Body.Close()
	b, e := io.ReadAll(resp.Body)
	if e != nil {

		return rb, fmt.Errorf("failed to read body: %w", err)
	}
	return b, nil
}

// skipProcessing checks redis to see if the line has been seen before.
// If it has, no data has changed and processing is stopped for that line.
func (c *Collector) skipProcessing(ctx context.Context, b []byte, reportType string) (bool, error) {
	nv := reportType + string(b)
	nv = strings.Replace(nv, " ", "", -1)
	nv = strings.Replace(nv, ",", "", -1)

	keyExists, err := c.redis.Exists(ctx, nv).Result()
	if err != nil {

		return false, fmt.Errorf("failed to check redis: %w", err)
	}
	if keyExists > 0 {

		return true, nil
	}

	if err := c.redis.Set(ctx, nv, 0, 0).Err(); err != nil {

		c.logger.Debug("error from redis", "set error", err.Error())
	}
	return false, nil
}

// CollectAndPublish  iterates of the NWS URLs and pulls down each report
// and forwards it to a kafka topic.  A header is added to identify the
// report type easily enough. Each line is iterated over and checked against a cache to
// see if this has already been seen.  If so, it is not processed.
func (c *Collector) CollectAndPublish(ctx context.Context) error {

	for _, url := range c.collectionURLs {

		body, err := c.collect(ctx, url.URL, c.logger)
		if err != nil {

			continue
		}
		lines := strings.Split(string(body), "\n")
		var skipped = 0

		ttl := 0

		for i, line := range lines {
			line = strings.TrimSpace(line)
			if i == 0 || line == "" {

				continue
			}
			ttl++

			skip, err := c.skipProcessing(ctx, []byte(line), url.Type)
			if err != nil {

				continue

			}

			if skip {
				skipped++

				continue
			}
			err = c.producer.WriteMessages(ctx, kafka.Message{Value: []byte(line), Headers: []kafka.Header{{
				Key:   "reportType",
				Value: []byte(url.Type),
			}}})
			if err != nil {
				c.logger.Debug("error writing message", "error", err, "line", line, "topic", c.topic)

				continue
			}

			c.logger.Debug("message write successful", "line", line)

			continue
		}

		c.logger.Info("processed report", "type", url.Type, "skipped", skipped, "processed", ttl-skipped, "total", ttl)
	}

	return nil
}

// Poll maintains the continued processing of reports at the specified interval.
func (c *Collector) Poll(ctx context.Context, interval time.Duration) {

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
Loop:
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.CollectAndPublish(ctx); err != nil {

				break Loop
			}

		}
	}
	<-ctx.Done()
}
