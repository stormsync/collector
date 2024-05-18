package collector

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	otelkafkakonsumer "github.com/Trendyol/otel-kafka-konsumer"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
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
	writer          *otelkafkakonsumer.Writer
	lastHailHash    string
	lastTornadoHash string
	lastWindHash    string
	hasher          hash.Hash
}

type Config struct {
	JaegerAddress string
	KafkaAddress  string
	KafkaUser     string
	KafkaPassword string
	URLsToPoll    map[ReportType]string
}

// NewCollector generates a new collector that has a kafka writer configured and ready to write.
func NewCollector(address, topic, user, pw string, collectionURLs map[ReportType]string) (*Collector, error) {
	tp := initJaegerTracer("http://localhost:14268/api/traces")
	mechanism, _ := scram.Mechanism(scram.SHA256, user, pw)
	// writer, err := otelkafkakonsumer.NewWriter(
	// &kafka.Writer{
	// 	Addr:  kafka.TCP(address),
	// 	Topic: topic,
	// 	Transport: &kafka.Transport{
	// 		SASL: mechanism,
	// 		TLS:  &tls.Config{},
	writer, err := otelkafkakonsumer.NewWriter(
		&kafka.Writer{
			Addr: kafka.TCP(address),
			Transport: &kafka.Transport{
				SASL: mechanism,
				TLS:  &tls.Config{},
			}},
		otelkafkakonsumer.WithTracerProvider(tp),
		otelkafkakonsumer.WithPropagator(propagation.TraceContext{}),
		otelkafkakonsumer.WithAttributes(
			[]attribute.KeyValue{
				semconv.MessagingDestinationKindTopic,
				semconv.MessagingKafkaClientIDKey.String("opentel-cg"),
			},
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create new kafka writer: %w", err)
	}
	return &Collector{
		topic:          topic,
		collectionURLs: collectionURLs,
		writer:         writer,
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
func (c *Collector) skipProcessing(ctx context.Context, b []byte, reportType ReportType) bool {
	tr := otel.Tracer("skip-processing-check")
	_, span := tr.Start(ctx, "generate-hash")
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
	span.End()
	return skip
}

// CollectAndPublish CollectAnPublish iterates of the NWS URLs and pulls down each report
// and forwards it to a kafka topic.  A header is added to identify the
// report type easily enough.
func (c *Collector) CollectAndPublish(ctx context.Context) error {
	tr := otel.Tracer("collect-and-publish")
	parentCtx, span := tr.Start(ctx, "collect-reports")
	defer span.End()

	for rt, url := range c.collectionURLs {
		bdy, err := collect(url)
		if err != nil {
			log.Println("failed to collect furl: ", err)
			span.RecordError(fmt.Errorf("failed to collect url: %w", err))
			continue
		}

		if c.skipProcessing(parentCtx, bdy, rt) {
			log.Println("skipped processing for ", rt.String())
			span.AddEvent(fmt.Sprintln("skipped processing for ", rt.String()))
			continue
		}

		msg := kafka.Message{Value: bdy, Headers: []kafka.Header{{
			Key:   "reportType",
			Value: []byte(rt.String()),
		}}}

		msgCtx := c.writer.TraceConfig.Propagator.Extract(parentCtx, otelkafkakonsumer.NewMessageCarrier(&msg))
		log.Println("sending message for ", rt.String())
		err = c.writer.WriteMessage(msgCtx, msg)
		if err != nil {
			span.RecordError(fmt.Errorf("failed to write message: %w", err))
			continue
		}

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
			log.Println("running collection job")
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

func initJaegerTracer(url string) *trace.TracerProvider {
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		log.Fatalf("Err initializing jaeger instance %v", err)
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("trace-demo"),
			attribute.String("environment", "prod"),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp
}
