package outbox

import (
	"context"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func TestNewDefaultPublisher(t *testing.T) {
	logger := zap.NewNop()
	publisher := NewDefaultPublisher(logger)

	if publisher == nil {
		t.Fatal("Expected non-nil publisher")
	}

	if publisher.logger != logger {
		t.Error("Expected logger to match")
	}
}

func TestDefaultPublisherPublish(t *testing.T) {
	publisher := NewDefaultPublisher(zap.NewNop())
	event := EventRecord{
		EventID: "test-event-id",
	}

	ctx := context.Background()
	err := publisher.Publish(ctx, event)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestDefaultKafkaConfig(t *testing.T) {
	config := DefaultKafkaConfig()

	if len(config.Brokers) != 1 || config.Brokers[0] != "localhost:9092" {
		t.Errorf("Expected brokers [localhost:9092], got %v", config.Brokers)
	}

	if config.Topic != "outbox-events" {
		t.Errorf("Expected topic 'outbox-events', got %s", config.Topic)
	}

	if config.BatchSize != 1 {
		t.Errorf("Expected BatchSize 1, got %d", config.BatchSize)
	}

	if config.BatchBytes != 1048576 {
		t.Errorf("Expected BatchBytes 1048576, got %d", config.BatchBytes)
	}

	if config.BatchTimeout != 10*time.Millisecond {
		t.Errorf("Expected BatchTimeout 10ms, got %v", config.BatchTimeout)
	}

	if config.Async != false {
		t.Error("Expected Async false")
	}

	if config.RequiredAcks != kafka.RequireAll {
		t.Errorf("Expected RequiredAcks RequireAll, got %v", config.RequiredAcks)
	}

	if config.Compression != kafka.Snappy {
		t.Errorf("Expected Compression Snappy, got %v", config.Compression)
	}

	if config.WriteTimeout != 10*time.Second {
		t.Errorf("Expected WriteTimeout 10s, got %v", config.WriteTimeout)
	}

	if config.ReadTimeout != 10*time.Second {
		t.Errorf("Expected ReadTimeout 10s, got %v", config.ReadTimeout)
	}

	if config.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts 3, got %d", config.MaxAttempts)
	}

	if config.ErrorLogger != nil {
		t.Error("Expected ErrorLogger to be nil")
	}

	if config.Logger != nil {
		t.Error("Expected Logger to be nil")
	}
}

func TestNewKafkaPublisher(t *testing.T) {
	logger := zap.NewNop()
	publisher := NewKafkaPublisher(logger)

	if publisher == nil {
		t.Fatal("Expected non-nil publisher")
	}

	if publisher.logger != logger {
		t.Error("Expected logger to match")
	}

	if publisher.writer == nil {
		t.Fatal("Expected non-nil writer")
	}

	if publisher.config.Topic != "outbox-events" {
		t.Errorf("Expected default topic 'outbox-events', got %s", publisher.config.Topic)
	}
}

func TestNewKafkaPublisherWithConfig(t *testing.T) {
	logger := zap.NewNop()
	config := KafkaConfig{
		Brokers:      []string{"localhost:9092", "localhost:9093"},
		Topic:        "custom-topic",
		BatchSize:    10,
		BatchBytes:   2048576,
		BatchTimeout: 20 * time.Millisecond,
		Async:        true,
		RequiredAcks: kafka.RequireOne,
		Compression:  kafka.Gzip,
		WriteTimeout: 20 * time.Second,
		ReadTimeout:  20 * time.Second,
		MaxAttempts:  5,
	}

	publisher := NewKafkaPublisherWithConfig(logger, config)

	if publisher == nil {
		t.Fatal("Expected non-nil publisher")
	}

	if publisher.logger != logger {
		t.Error("Expected logger to match")
	}

	if publisher.writer == nil {
		t.Fatal("Expected non-nil writer")
	}

	if publisher.config.Topic != config.Topic {
		t.Errorf("Expected topic %s, got %s", config.Topic, publisher.config.Topic)
	}

	if publisher.config.BatchSize != config.BatchSize {
		t.Errorf("Expected BatchSize %d, got %d", config.BatchSize, publisher.config.BatchSize)
	}
}

func TestKafkaPublisherClose(t *testing.T) {
	publisher := NewKafkaPublisher(zap.NewNop())

	err := publisher.Close()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	err = publisher.Close()
	if err != nil {
		t.Errorf("Expected no error on second close, got %v", err)
	}
}

func TestKafkaPublisherCloseNilWriter(t *testing.T) {
	publisher := &KafkaPublisher{
		logger: zap.NewNop(),
		writer: nil,
	}

	err := publisher.Close()
	if err != nil {
		t.Errorf("Expected no error with nil writer, got %v", err)
	}
}

func TestBuildKafkaHeaders(t *testing.T) {
	publisher := NewKafkaPublisher(zap.NewNop())

	event := EventRecord{
		EventID:       "test-event-id",
		EventType:     "test-event-type",
		AggregateType: "test-aggregate-type",
		AggregateID:   "test-aggregate-id",
		TraceID:       "test-trace-id",
		SpanID:        "test-span-id",
	}

	headers := publisher.buildKafkaHeaders(event)

	expectedHeaders := map[string]string{
		"event_id":       "test-event-id",
		"event_type":     "test-event-type",
		"aggregate_type": "test-aggregate-type",
		"aggregate_id":   "test-aggregate-id",
		"trace_id":       "test-trace-id",
		"span_id":        "test-span-id",
	}

	if len(headers) != len(expectedHeaders) {
		t.Errorf("Expected %d headers, got %d", len(expectedHeaders), len(headers))
	}

	for _, header := range headers {
		expectedValue, exists := expectedHeaders[header.Key]
		if !exists {
			t.Errorf("Unexpected header key: %s", header.Key)
			continue
		}

		if string(header.Value) != expectedValue {
			t.Errorf("Expected header %s value %s, got %s", header.Key, expectedValue, string(header.Value))
		}
	}
}

func TestBuildKafkaHeadersWithoutTraceInfo(t *testing.T) {
	publisher := NewKafkaPublisher(zap.NewNop())

	event := EventRecord{
		EventID:       "test-event-id",
		EventType:     "test-event-type",
		AggregateType: "test-aggregate-type",
		AggregateID:   "test-aggregate-id",
		TraceID:       "",
		SpanID:        "",
	}

	headers := publisher.buildKafkaHeaders(event)

	expectedHeaders := map[string]string{
		"event_id":       "test-event-id",
		"event_type":     "test-event-type",
		"aggregate_type": "test-aggregate-type",
		"aggregate_id":   "test-aggregate-id",
	}

	if len(headers) != len(expectedHeaders) {
		t.Errorf("Expected %d headers, got %d", len(expectedHeaders), len(headers))
	}

	for _, header := range headers {
		expectedValue, exists := expectedHeaders[header.Key]
		if !exists {
			t.Errorf("Unexpected header key: %s", header.Key)
			continue
		}

		if string(header.Value) != expectedValue {
			t.Errorf("Expected header %s value %s, got %s", header.Key, expectedValue, string(header.Value))
		}
	}
}
