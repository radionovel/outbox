package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaProducer представляет продюсера Kafka с настройками идемпотентности
type KafkaProducer struct {
	writer *kafka.Writer
	topic  string
}

// NewKafkaProducer создает новый Kafka продюсер с настройками идемпотентности
func NewKafkaProducer(brokers []string, topic string) *KafkaProducer {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		RequiredAcks: kafka.RequireAll, // Идемпотентный продюсер
		Async:        false,             // Синхронная отправка для гарантии доставки
		Compression:  kafka.Snappy,      // Сжатие для оптимизации
		BatchTimeout: 10 * time.Millisecond,
		BatchSize:    100,
		// Настройки для идемпотентности
		AllowAutoTopicCreation: true,
		MaxAttempts:            3,
		ReadTimeout:            10 * time.Second,
		WriteTimeout:           10 * time.Second,
	}

	return &KafkaProducer{
		writer: writer,
		topic:  topic,
	}
}

// PublishEvent отправляет событие в Kafka с ключом aggregate_id для гарантии порядка
func (p *KafkaProducer) PublishEvent(ctx context.Context, event OutboxEvent) error {
	// Сериализуем событие в JSON
	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Создаем сообщение с ключом aggregate_id для гарантии порядка по сущности
	message := kafka.Message{
		Key:   []byte(event.AggregateID),
		Value: eventBytes,
		Headers: []kafka.Header{
			{Key: "event_id", Value: []byte(event.EventID)},
			{Key: "event_type", Value: []byte(event.EventType)},
			{Key: "aggregate_type", Value: []byte(event.AggregateType)},
			{Key: "timestamp", Value: []byte(time.Now().Format(time.RFC3339))},
		},
	}

	// Отправляем сообщение
	err = p.writer.WriteMessages(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to publish event to Kafka: %w", err)
	}

	return nil
}

// PublishEventsBatch отправляет пакет событий в Kafka
func (p *KafkaProducer) PublishEventsBatch(ctx context.Context, events []OutboxEvent) error {
	if len(events) == 0 {
		return nil
	}

	messages := make([]kafka.Message, len(events))
	for i, event := range events {
		eventBytes, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to marshal event %d: %w", i, err)
		}

		messages[i] = kafka.Message{
			Key:   []byte(event.AggregateID),
			Value: eventBytes,
			Headers: []kafka.Header{
				{Key: "event_id", Value: []byte(event.EventID)},
				{Key: "event_type", Value: []byte(event.EventType)},
				{Key: "aggregate_type", Value: []byte(event.AggregateType)},
				{Key: "timestamp", Value: []byte(time.Now().Format(time.RFC3339))},
			},
		}
	}

	err := p.writer.WriteMessages(ctx, messages...)
	if err != nil {
		return fmt.Errorf("failed to publish events batch to Kafka: %w", err)
	}

	return nil
}

// Close закрывает соединение с Kafka
func (p *KafkaProducer) Close() error {
	return p.writer.Close()
}

// HealthCheck проверяет доступность Kafka
func (p *KafkaProducer) HealthCheck(ctx context.Context) error {
	// Простая проверка - пытаемся получить метаданные топика
	conn, err := kafka.DialLeader(ctx, "tcp", p.writer.Addr.String(), p.topic, 0)
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka: %w", err)
	}
	defer conn.Close()

	return nil
}
