package collector

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

//go:generate stringer -type=ReportType
type ReportType int

const (
	Hail ReportType = iota
	Wind
	Tornado
)

type Collector struct {
	topic          string
	collectionURLs map[ReportType]string
	producer       *kafka.Writer
	lineHashes     map[string]struct{}
	hasher         hash.Hash
	logger         *slog.Logger
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
func NewCollector(collectionURLs map[ReportType]string, rc *redis.Client, kw *kafka.Writer, logger *slog.Logger) (*Collector, error) {
	if logger == nil {
		log.Fatal("logger can't be nil")
	}
	return &Collector{
		topic:          kw.Topic,
		collectionURLs: collectionURLs,
		producer:       kw,
		hasher:         sha256.New(),
		logger:         logger,
		redis:          rc,
		lineHashes:     make(map[string]struct{}),
	}, nil
}

// collect does the work of making the get request and checking the resp for validity.
func collect(url string, logger *slog.Logger) ([]byte, error) {
	var rb []byte
	logger.Debug("collect()", "url", url)
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
	// logger.Debug("response body", "body", string(b))
	return b, nil
}

// skipProcessing checks redis to see if the line has been seen before.
// If it has, no data has changed and processing is stopped for that line.
func (c *Collector) skipProcessing(ctx context.Context, b []byte, reportType ReportType) (bool, error) {
	nv := reportType.String() + string(b)
	nv = strings.Replace(nv, " ", "", -1)
	nv = strings.Replace(nv, ",", "", -1)

	keyExists, err := c.redis.Exists(ctx, nv).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check redis: %w", err)
	}
	if keyExists > 0 {
		// c.logger.Debug("skipping line", "match found in cache", string(b))
		return true, nil
	}

	if err := c.redis.Set(ctx, nv, 0, 0).Err(); err != nil {
		c.logger.Debug("error from redis", "set error", err.Error())
	}
	return false, nil
}

// 1: 007fc93e500974bff4c6df4962460248192b4264dbe998d86b2d51150647f815

// CollectAndPublish  iterates of the NWS URLs and pulls down each report
// and forwards it to a kafka topic.  A header is added to identify the
// report type easily enough. Each line is iterated over and checked against a cache to
// see if this has already been seen.  If so, it is not processed.
func (c *Collector) CollectAndPublish(ctx context.Context) error {
	for rt, url := range c.collectionURLs {
		c.logger.Debug("Starting collections", "type", rt.String())
		body, err := collect(url, c.logger)
		if err != nil {
			return fmt.Errorf("failed to collect url: %w", err)
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
			skip, err := c.skipProcessing(ctx, []byte(line), rt)
			if err != nil {
				return fmt.Errorf("skipProcessing failed: %w", err)
			}

			if skip {
				skipped++
				continue
			}

			err = c.producer.WriteMessages(ctx, kafka.Message{Value: []byte(line), Headers: []kafka.Header{{
				Key:   "reportType",
				Value: []byte(rt.String()),
			}}})

			if err != nil {
				return fmt.Errorf("failed to write message to topic %s: %w", c.topic, err)
			}
			c.logger.Debug("message write successful", "line", line)
			continue
		}
		c.logger.Info("processed report", "type", rt.String(), "skipped", skipped, "processed", ttl-skipped, "total", ttl)
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
				c.logger.Error("polling error", "error", err)
				break Loop
			}

		}
	}
	<-ctx.Done()
}
