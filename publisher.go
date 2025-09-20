package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.uber.org/zap"
)

// NopPublisher is a no-op publisher that does nothing.
type NopPublisher struct {
	logger *zap.Logger
}

// NewDefaultPublisher creates a new NopPublisher.
func NewDefaultPublisher(logger *zap.Logger) *NopPublisher {
	return &NopPublisher{
		logger: logger,
	}
}

// Publish does nothing and returns nil.
func (p *NopPublisher) Publish(_ context.Context, _ EventRecord) error {
	return nil
}

// KafkaPublisher is a publisher that sends events to Kafka using the Confluent Kafka library.
type KafkaPublisher struct {
	logger   *zap.Logger
	producer *kafka.Producer
	config   KafkaConfig
}

// KafkaConfig holds the configuration for the Kafka publisher.
type KafkaConfig struct {
	Topic         string
	ProducerProps kafka.ConfigMap
}

// DefaultKafkaConfig returns a default configuration for the Kafka publisher.
func DefaultKafkaConfig() KafkaConfig {
	return KafkaConfig{
		Topic: "outbox-events",
		ProducerProps: kafka.ConfigMap{
			"bootstrap.servers": "localhost:9092",
			"acks":              "all",
			"retries":           3,
			"linger.ms":         10,
			"compression.type":  "snappy",
		},
	}
}

// NewKafkaPublisher creates a new KafkaPublisher with the default configuration.
func NewKafkaPublisher(logger *zap.Logger) (*KafkaPublisher, error) {
	config := DefaultKafkaConfig()
	return NewKafkaPublisherWithConfig(logger, config)
}

// NewKafkaPublisherWithConfig creates a new KafkaPublisher with the given configuration.
func NewKafkaPublisherWithConfig(logger *zap.Logger, config KafkaConfig) (*KafkaPublisher, error) {
	producer, err := kafka.NewProducer(&config.ProducerProps)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka producer: %w", err)
	}

	p := &KafkaPublisher{
		logger:   logger,
		producer: producer,
		config:   config,
	}

	go p.handleDeliveryReports()

	return p, nil
}

// Publish sends an event to Kafka.
func (p *KafkaPublisher) Publish(ctx context.Context, event EventRecord) error {
	topic := event.Topic
	if topic == "" {
		topic = p.config.Topic
	}

	p.logger.Info("Publishing event to Kafka",
		zap.String("event_id", event.EventID),
		zap.String("event_type", event.EventType),
		zap.String("topic", topic),
	)

	message := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
		Key:            []byte(event.AggregateID),
		Value:          event.Payload,
		Headers:        p.buildKafkaHeaders(event),
		Timestamp:      time.Now(),
	}

	return p.producer.Produce(message, nil)
}

// Close closes the Kafka producer.
func (p *KafkaPublisher) Close() error {
	p.logger.Info("Closing kafka producer")
	p.producer.Flush(15 * 1000)
	p.producer.Close()
	return nil
}

// handleDeliveryReports handles delivery reports from Kafka.
func (p *KafkaPublisher) handleDeliveryReports() {
	for e := range p.producer.Events() {
		switch ev := e.(type) {
		case *kafka.Message:
			if ev.TopicPartition.Error != nil {
				p.logger.Error("Delivery failed",
					zap.String("topic", *ev.TopicPartition.Topic),
					zap.Error(ev.TopicPartition.Error),
				)
			} else {
				p.logger.Debug("Successfully delivered message",
					zap.String("topic", *ev.TopicPartition.Topic),
					zap.Int32("partition", ev.TopicPartition.Partition),
					zap.Any("offset", ev.TopicPartition.Offset),
				)
			}
		case kafka.Error:
			p.logger.Error("Kafka error", zap.Error(ev))
		default:
			p.logger.Debug("Ignored kafka event", zap.Any("event", ev))
		}
	}
}

// buildKafkaHeaders builds Kafka headers from an EventRecord.
func (p *KafkaPublisher) buildKafkaHeaders(event EventRecord) []kafka.Header {
	headers := []kafka.Header{
		{Key: "event_id", Value: []byte(event.EventID)},
		{Key: "event_type", Value: []byte(event.EventType)},
		{Key: "aggregate_type", Value: []byte(event.AggregateType)},
		{Key: "aggregate_id", Value: []byte(event.AggregateID)},
	}

	if event.TraceID != "" {
		headers = append(headers, kafka.Header{Key: "trace_id", Value: []byte(event.TraceID)})
	}
	if event.SpanID != "" {
		headers = append(headers, kafka.Header{Key: "span_id", Value: []byte(event.SpanID)})
	}

	return headers
}
