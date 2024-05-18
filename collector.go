package collector

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
)

//go:generate stringer -type=ReportType
type ReportType int

const (
	Hail ReportType = iota
	Wind
	Tornado
)

type Collector struct {
	topic           string
	collectionURLs  map[ReportType]string
	producer        kafka.Writer
	lastHailHash    string
	lastTornadoHash string
	lastWindHash    string
	hasher          hash.Hash
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
func NewCollector(address, topic, user, pw string, collectionURLs map[ReportType]string) (*Collector, error) {
	mechanism, _ := scram.Mechanism(scram.SHA256, user, pw)
	w := kafka.Writer{
		Addr:  kafka.TCP(address),
		Topic: topic,
		Transport: &kafka.Transport{
			SASL: mechanism,
			TLS:  &tls.Config{},
		},
	}

	return &Collector{
		topic:          topic,
		collectionURLs: collectionURLs,
		producer:       w,
		hasher:         sha256.New(),
	}, nil
}

// collect does the work of making the get request and checking the resp for validity.
func collect(url string) ([]byte, error) {
	var rb []byte
	resp, err := http.Get(url)
	if err != nil {
		return rb, fmt.Errorf("error making get request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return rb, fmt.Errorf("non-200 status code returned: %s", resp.Status)
	}

	defer resp.Body.Close()
	b, e := io.ReadAll(resp.Body)
	if e != nil {
		return rb, fmt.Errorf("failed to read body: %w", err)
	}
	return b, nil
}

// skipProcessing generates a hash of the incoming storm report and compares
// it to the hash of the previous report of the same type (hail, wind, tornado).
// If the hashes match, no data has changed and processing is stopped for that report.
func (c *Collector) skipProcessing(b []byte, reportType ReportType) bool {
	c.hasher.Write(b)
	sha := c.hasher.Sum(nil)
	shaStr := hex.EncodeToString(sha)
	skip := false
	switch reportType {
	case Wind:
		if strings.EqualFold(shaStr, c.lastWindHash) {
			// log.Println("wind content has not changed, skipping    ", shaStr, c.lastWindHash)
			skip = true
		} else {
			c.lastWindHash = shaStr
		}
	case Hail:
		if strings.EqualFold(shaStr, c.lastHailHash) {
			// log.Println("hail content has not changed, skipping    ", shaStr, c.lastHailHash)
			skip = true
		} else {
			c.lastHailHash = shaStr
		}
	case Tornado:
		if strings.EqualFold(shaStr, c.lastTornadoHash) {
			// log.Println("tornado content has not changed, skipping    ", shaStr, c.lastTornadoHash)
			skip = true
		} else {
			c.lastTornadoHash = shaStr
		}
	}
	return skip
}

// CollectAndPublish CollectAnPublish iterates of the NWS URLs and pulls down each report
// and forwards it to a kafka topic.  A header is added to identify the
// report type easily enough.
func (c *Collector) CollectAndPublish(ctx context.Context) error {
	defer c.producer.Close()
	for rt, url := range c.collectionURLs {
		bdy, err := collect(url)
		if err != nil {
			return fmt.Errorf("failed to collect url: %w", err)
		}
		if c.skipProcessing(bdy, rt) {
			continue
		}

		fmt.Println("writing message ")
		err = c.producer.WriteMessages(ctx, kafka.Message{Value: bdy, Headers: []kafka.Header{{
			Key:   "reportType",
			Value: []byte(rt.String()),
		}}})
		if err != nil {
			return err
		}
		fmt.Print("wrote \n", rt.String())
		continue
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
				if strings.Contains(err.Error(), "SASL Authentication failed") {
					log.Println("Kafka Auth Failure: ", err.Error())
					break Loop
				}

			}
		}
		<-ctx.Done()
	}
}
