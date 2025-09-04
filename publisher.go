package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type NopPublisher struct {
	logger *zap.Logger
}

func NewDefaultPublisher(logger *zap.Logger) *NopPublisher {
	return &NopPublisher{
		logger: logger,
	}
}

func (p *NopPublisher) Publish(ctx context.Context, event EventRecord) error {
	return nil
}

type KafkaPublisher struct {
	logger *zap.Logger
	writer *kafka.Writer
	config KafkaConfig
}

type KafkaConfig struct {
	Brokers      []string
	Topic        string
	Balancer     kafka.Balancer
	BatchSize    int
	BatchBytes   int64
	BatchTimeout time.Duration
	Async        bool
	RequiredAcks kafka.RequiredAcks
	Compression  kafka.Compression
	WriteTimeout time.Duration
	ReadTimeout  time.Duration
	MaxAttempts  int
	ErrorLogger  kafka.Logger
	Logger       kafka.Logger
}

func DefaultKafkaConfig() KafkaConfig {
	return KafkaConfig{
		Brokers:      []string{"localhost:9092"},
		Topic:        "outbox-events",
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    1,
		BatchBytes:   1048576, // 1MB
		BatchTimeout: 10 * time.Millisecond,
		Async:        false,
		RequiredAcks: kafka.RequireAll,
		Compression:  kafka.Snappy,
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
		MaxAttempts:  3,
		ErrorLogger:  nil,
		Logger:       nil,
	}
}

func NewKafkaPublisher(logger *zap.Logger) *KafkaPublisher {
	config := DefaultKafkaConfig()
	return NewKafkaPublisherWithConfig(logger, config)
}

func NewKafkaPublisherWithConfig(logger *zap.Logger, config KafkaConfig) *KafkaPublisher {
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(config.Brokers...),
		Balancer:               config.Balancer,
		BatchSize:              config.BatchSize,
		BatchBytes:             config.BatchBytes,
		BatchTimeout:           config.BatchTimeout,
		Async:                  config.Async,
		RequiredAcks:           config.RequiredAcks,
		Compression:            config.Compression,
		WriteTimeout:           config.WriteTimeout,
		ReadTimeout:            config.ReadTimeout,
		MaxAttempts:            config.MaxAttempts,
		ErrorLogger:            config.ErrorLogger,
		AllowAutoTopicCreation: true,
		Logger:                 config.Logger,
	}

	return &KafkaPublisher{
		logger: logger,
		writer: writer,
		config: config,
	}
}

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

	message := kafka.Message{
		Topic:   topic,
		Key:     []byte(event.AggregateID),
		Value:   event.Payload,
		Headers: p.buildKafkaHeaders(event),
		Time:    time.Now(),
	}

	err := p.writer.WriteMessages(ctx, message)
	if err != nil {
		p.logger.Error("Failed to publish event to Kafka",
			zap.String("event_id", event.EventID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to publish event to Kafka: %w", err)
	}

	p.logger.Info("Successfully published event to Kafka",
		zap.String("event_id", event.EventID),
		zap.String("topic", topic),
	)

	return nil
}

func (p *KafkaPublisher) Close() error {
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}

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
