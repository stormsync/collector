package collector

import (
	"context"
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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	_ "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

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
	collectionURLs map[ReportType]string
	producer       *kafka.Writer
	lineHashes     map[string]struct{}
	hasher         hash.Hash
	logger         *slog.Logger
	tracer         trace.Tracer
	redis          *redis.Client
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
func NewCollector(ctx context.Context, c config.Config, tracer trace.Tracer, l *slog.Logger) (*Collector, error) {
	if l == nil {
		return nil, errors.New("logger cannot be nil")
	}

	if tracer == nil {
		return nil, errors.New("tracer cannot be nil")
	}

	if c.Services.Collector.CollectionURLs == nil {
		return nil, errors.New("collection urls cannot be nil")
	}

	if c.Services.Kafka.Topic == "" {
		return nil, errors.New("kafka topic is required")
	}
	kw, err := newKafkaWriter(c.Services.Kafka.Host, c.Services.Kafka.Port, c.Services.Kafka.Topic, c.Services.Kafka.User, c.Services.Kafka.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to create a kafka writer: %w", err)
	}

	rc, err := newRedisClient(ctx, c.Services.Redis.Host+":"+c.Services.Redis.Port, c.Services.Redis.User, c.Services.Redis.Password, c.Services.Redis.DB)
	if err != nil {
		return nil, fmt.Errorf("failed to create a redis client: %w", err)
	}

	// TODO: these are buired here...need to move out and make configurable?
	var (
		WindURL    = "https://www.spc.noaa.gov/climo/reports/today_wind.csv"
		HailURL    = "https://www.spc.noaa.gov/climo/reports/today_hail.csv"
		TornadoURL = "https://www.spc.noaa.gov/climo/reports/today_torn.csv"
	)

	urlMap := map[ReportType]string{
		Wind:    WindURL,
		Hail:    HailURL,
		Tornado: TornadoURL,
	}

	return &Collector{
		topic:          kw.Topic,
		collectionURLs: urlMap,
		producer:       kw,
		logger:         l,
		tracer:         tracer,
		redis:          rc,
	}, nil
}

func newKafkaWriter(host, port, topic, user, pw string) (*kafka.Writer, error) {
	// mechanism, err := scram.Mechanism(scram.SHA256, "", "")
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create scram.Mechanism for auth: %w", err)
	// }
	w := &kafka.Writer{
		// Addr:  kafka.TCP("kafka-1:9092"),
		Addr:  kafka.TCP("organic-ray-9236-us1-kafka.upstash.io:9092"),
		Topic: "raw-weather-report",
		// Transport: &kafka.Transport{
		// 	SASL: mechanism,
		// 	TLS:  &tls.Config{},
		// },
	}
	return w, nil
}

func newRedisClient(ctx context.Context, address, user, password string, db int) (*redis.Client, error) {
	opt, err := redis.ParseURL(fmt.Sprintf("rediss://%s:%s@%s", user, password, address))
	c := redis.NewClient(opt)
	if err != nil {
		return nil, err
	}

	return c, nil
}

// collect does the work of making the get request and checking the resp for validity.
func (c *Collector) collect(ctx context.Context, url string, logger *slog.Logger) ([]byte, error) {
	ctx, span := c.tracer.Start(ctx, "Collector.collect()")
	defer span.End()
	span.SetAttributes(attribute.String("url", url))

	var rb []byte

	resp, err := http.Get(url)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintln("http.Get failed for ", url))
		span.AddEvent(fmt.Sprint("error making get request: %w", err))
		return rb, fmt.Errorf("error making get request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		span.AddEvent(fmt.Sprintf("non-200 status code returned: %s", resp.Status))
		span.SetStatus(codes.Error, "non-200 status returned")
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
func (c *Collector) skipProcessing(ctx context.Context, b []byte, reportType ReportType) (bool, error) {
	ctx, span := c.tracer.Start(ctx, "Checking for Duplicates")
	defer span.End()
	span.SetAttributes(attribute.String("type", reportType.String()))
	nv := reportType.String() + string(b)
	nv = strings.Replace(nv, " ", "", -1)
	nv = strings.Replace(nv, ",", "", -1)

	keyExists, err := c.redis.Exists(ctx, nv).Result()
	if err != nil {
		span.AddEvent("failed to check redis")
		return false, fmt.Errorf("failed to check redis: %w", err)
	}
	if keyExists > 0 {
		span.AddEvent("duplicate key found, skipping")
		return true, nil
	}

	if err := c.redis.Set(ctx, nv, 0, 0).Err(); err != nil {
		span.AddEvent("unique record, added key to redis")
		c.logger.Debug("error from redis", "set error", err.Error())
	}
	return false, nil
}

// CollectAndPublish  iterates of the NWS URLs and pulls down each report
// and forwards it to a kafka topic.  A header is added to identify the
// report type easily enough. Each line is iterated over and checked against a cache to
// see if this has already been seen.  If so, it is not processed.
func (c *Collector) CollectAndPublish(ctx context.Context) error {
	ctx, span := c.tracer.Start(ctx, "collector.CollectAndPublish")
	defer span.End()

	for rt, url := range c.collectionURLs {
		span.AddEvent(fmt.Sprintf("collecting from %s", url))
		body, err := c.collect(ctx, url, c.logger)
		if err != nil {
			span.RecordError(err)
			continue
		}
		lines := strings.Split(string(body), "\n")
		var skipped = 0

		ttl := 0

		for i, line := range lines {
			line = strings.TrimSpace(line)
			if i == 0 || line == "" {
				span.AddEvent("Nothing to process")
				continue
			}
			ttl++
			ictx, iSpan := c.tracer.Start(ctx, "processing line")

			skip, err := c.skipProcessing(ictx, []byte(line), rt)
			if err != nil {
				iSpan.RecordError(err)
				iSpan.End()
				continue

			}

			if skip {
				skipped++
				iSpan.End()
				continue
			}
			err = c.producer.WriteMessages(ictx, kafka.Message{Value: []byte(line), Headers: []kafka.Header{{
				Key:   "reportType",
				Value: []byte(rt.String()),
			}}})
			if err != nil {
				c.logger.Debug("error writing message", "error", err, "line", line, "topic", c.topic)
				iSpan.RecordError(err)
				iSpan.SetStatus(codes.Error, err.Error())
				iSpan.End()
				continue
			}
			iSpan.AddEvent("message write successful")
			iSpan.SetAttributes(attribute.String("line", line))
			c.logger.Debug("message write successful", "line", line)
			iSpan.End()
			continue
		}

		span.AddEvent("report processing complete")
		c.logger.Info("processed report", "type", rt.String(), "skipped", skipped, "processed", ttl-skipped, "total", ttl)
	}

	return nil
}

// Poll maintains the continued processing of reports at the specified interval.
func (c *Collector) Poll(ctx context.Context, interval time.Duration) {
	ctx, span := c.tracer.Start(ctx, "collector.Poll()")
	defer span.End()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
Loop:
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.CollectAndPublish(ctx); err != nil {
				span.RecordError(err)
				break Loop
			}

		}
	}
	<-ctx.Done()
}
