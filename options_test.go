package outbox

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestWithBatchSize(t *testing.T) {
	opts := &dispatcherOptions{}
	WithBatchSize(50)(opts)

	if opts.batchSize != 50 {
		t.Errorf("Expected batchSize 50, got %d", opts.batchSize)
	}
}

func TestWithPollInterval(t *testing.T) {
	opts := &dispatcherOptions{}
	interval := 5 * time.Second
	WithPollInterval(interval)(opts)

	if opts.pollInterval != interval {
		t.Errorf("Expected pollInterval %v, got %v", interval, opts.pollInterval)
	}
}

func TestWithMaxAttempts(t *testing.T) {
	opts := &dispatcherOptions{}
	WithMaxAttempts(5)(opts)

	if opts.maxAttempts != 5 {
		t.Errorf("Expected maxAttempts 5, got %d", opts.maxAttempts)
	}
}

func TestWithDeadLetterInterval(t *testing.T) {
	opts := &dispatcherOptions{}
	interval := 10 * time.Minute
	WithDeadLetterInterval(interval)(opts)

	if opts.deadLetterInterval != interval {
		t.Errorf("Expected deadLetterInterval %v, got %v", interval, opts.deadLetterInterval)
	}
}

func TestWithStuckEventTimeout(t *testing.T) {
	opts := &dispatcherOptions{}
	timeout := 15 * time.Minute
	WithStuckEventTimeout(timeout)(opts)

	if opts.stuckEventTimeout != timeout {
		t.Errorf("Expected stuckEventTimeout %v, got %v", timeout, opts.stuckEventTimeout)
	}
}

func TestWithStuckEventCheckInterval(t *testing.T) {
	opts := &dispatcherOptions{}
	interval := 3 * time.Minute
	WithStuckEventCheckInterval(interval)(opts)

	if opts.stuckEventCheckInterval != interval {
		t.Errorf("Expected stuckEventCheckInterval %v, got %v", interval, opts.stuckEventCheckInterval)
	}
}

func TestWithDeadLetterRetention(t *testing.T) {
	opts := &dispatcherOptions{}
	retention := 14 * 24 * time.Hour
	WithDeadLetterRetention(retention)(opts)

	if opts.deadLetterRetention != retention {
		t.Errorf("Expected deadLetterRetention %v, got %v", retention, opts.deadLetterRetention)
	}
}

func TestWithSentEventsRetention(t *testing.T) {
	opts := &dispatcherOptions{}
	retention := 48 * time.Hour
	WithSentEventsRetention(retention)(opts)

	if opts.sentEventsRetention != retention {
		t.Errorf("Expected sentEventsRetention %v, got %v", retention, opts.sentEventsRetention)
	}
}

func TestWithCleanupInterval(t *testing.T) {
	opts := &dispatcherOptions{}
	interval := 2 * time.Hour
	WithCleanupInterval(interval)(opts)

	if opts.cleanupInterval != interval {
		t.Errorf("Expected cleanupInterval %v, got %v", interval, opts.cleanupInterval)
	}
}

func TestWithBackoffStrategy(t *testing.T) {
	opts := &dispatcherOptions{}
	strategy := NewFixedBackoffStrategy(1 * time.Second)
	WithBackoffStrategy(strategy)(opts)

	if opts.backoffStrategy != strategy {
		t.Error("Expected backoffStrategy to match provided strategy")
	}
}

func TestWithPublisher(t *testing.T) {
	opts := &dispatcherOptions{}
	logger := zap.NewNop()
	publisher := NewDefaultPublisher(logger)
	WithPublisher(publisher)(opts)

	if opts.publisher != publisher {
		t.Error("Expected publisher to match provided publisher")
	}
}

func TestWithMetrics(t *testing.T) {
	opts := &dispatcherOptions{}
	metrics := NewNoOpMetricsCollector()
	WithMetrics(metrics)(opts)

	if opts.metrics != metrics {
		t.Error("Expected metrics to match provided metrics")
	}
}

func TestWithLogger(t *testing.T) {
	opts := &dispatcherOptions{}
	logger := zap.NewNop()
	WithLogger(logger)(opts)

	if opts.logger != logger {
		t.Error("Expected logger to match provided logger")
	}
}

func TestWithKafkaConfig(t *testing.T) {
	opts := &dispatcherOptions{}
	opts.logger = zap.NewNop()
	config := KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "test-topic",
	}
	WithKafkaConfig(config)(opts)

	if opts.publisher == nil {
		t.Fatal("Expected publisher to be set")
	}

	kafkaPublisher, ok := opts.publisher.(*KafkaPublisher)
	if !ok {
		t.Fatal("Expected KafkaPublisher type")
	}

	if kafkaPublisher.config.Topic != config.Topic {
		t.Errorf("Expected topic %s, got %s", config.Topic, kafkaPublisher.config.Topic)
	}
}

func TestMultipleOptions(t *testing.T) {
	opts := &dispatcherOptions{}
	logger := zap.NewNop()
	metrics := NewNoOpMetricsCollector()
	strategy := NewFixedBackoffStrategy(1 * time.Second)

	WithBatchSize(25)(opts)
	WithPollInterval(3 * time.Second)(opts)
	WithMaxAttempts(7)(opts)
	WithLogger(logger)(opts)
	WithMetrics(metrics)(opts)
	WithBackoffStrategy(strategy)(opts)

	if opts.batchSize != 25 {
		t.Errorf("Expected batchSize 25, got %d", opts.batchSize)
	}
	if opts.pollInterval != 3*time.Second {
		t.Errorf("Expected pollInterval 3s, got %v", opts.pollInterval)
	}
	if opts.maxAttempts != 7 {
		t.Errorf("Expected maxAttempts 7, got %d", opts.maxAttempts)
	}
	if opts.logger != logger {
		t.Error("Expected logger to match")
	}
	if opts.metrics != metrics {
		t.Error("Expected metrics to match")
	}
	if opts.backoffStrategy != strategy {
		t.Error("Expected backoffStrategy to match")
	}
}
