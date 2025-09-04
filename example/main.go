package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"

	"github.com/overtonx/outbox"
)

func main() {
	// Создаем логгер
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal("Failed to create logger:", err)
	}
	defer logger.Sync()

	// Подключаемся к базе данных
	db, err := sql.Open("mysql", "outbox_user:outbox_pass@tcp(localhost:3306)/outbox_db?parseTime=true")
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Проверяем соединение
	if err := db.Ping(); err != nil {
		logger.Fatal("Failed to ping database", zap.Error(err))
	}

	// Создаем таблицы outbox
	ctx := context.Background()
	if err := outbox.CreateOutboxTable(ctx, db); err != nil {
		logger.Fatal("Failed to create outbox tables", zap.Error(err))
	}
	logger.Info("Outbox tables created successfully")

	db.Exec("TRUNCATE outbox_events")
	db.Exec("TRUNCATE outbox_deadletters")

	// Создаем конфигурацию Kafka
	kafkaConfig := outbox.KafkaConfig{
		Brokers:      []string{"localhost:9092"},
		Topic:        "user-events",
		BatchSize:    10,
		BatchTimeout: 100 * time.Millisecond,
		Async:        false,
	}

	// Создаем dispatcher с настройками
	dispatcher := outbox.NewDispatcher(db,
		outbox.WithLogger(logger),
		outbox.WithKafkaConfig(kafkaConfig),
		outbox.WithBatchSize(5),
		outbox.WithPollInterval(1*time.Second),
		outbox.WithMaxAttempts(3),
	)

	// Запускаем dispatcher в отдельной горутине
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		dispatcher.Start(ctx)
	}()

	// Ждем немного, чтобы dispatcher запустился
	time.Sleep(2 * time.Second)

	// Симулируем создание событий
	logger.Info("Starting to create sample events...")
	createSampleEvents(ctx, db, logger)

	// Ждем сигнал завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Показываем метрики каждые 5 секунд
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			logger.Info("Received shutdown signal")
			cancel()
			dispatcher.Stop()
			return
		case <-ticker.C:
			metrics := dispatcher.GetMetrics()
			logger.Info("Dispatcher metrics", zap.Any("metrics", metrics))
		}
	}
}

func createSampleEvents(ctx context.Context, db *sql.DB, logger *zap.Logger) {
	// Начинаем транзакцию
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		logger.Error("Failed to begin transaction", zap.Error(err))
		return
	}
	defer tx.Rollback()

	// Создаем несколько событий
	events := []struct {
		eventID       string
		eventType     string
		aggregateType string
		aggregateID   string
		topic         string
		payload       map[string]interface{}
	}{
		{
			eventID:       "evt-001",
			eventType:     "UserCreated",
			aggregateType: "User",
			aggregateID:   "user-123",
			topic:         "user-events",
			payload: map[string]interface{}{
				"user_id":    "user-123",
				"email":      "john@example.com",
				"name":       "John Doe",
				"created_at": time.Now().Format(time.RFC3339),
			},
		},
		{
			eventID:       "evt-002",
			eventType:     "UserUpdated",
			aggregateType: "User",
			aggregateID:   "user-123",
			topic:         "user-events",
			payload: map[string]interface{}{
				"user_id":    "user-123",
				"email":      "john.doe@example.com",
				"name":       "John Doe",
				"updated_at": time.Now().Format(time.RFC3339),
			},
		},
		{
			eventID:       "evt-003",
			eventType:     "OrderCreated",
			aggregateType: "Order",
			aggregateID:   "order-456",
			topic:         "order-events",
			payload: map[string]interface{}{
				"order_id":   "order-456",
				"user_id":    "user-123",
				"amount":     99.99,
				"currency":   "USD",
				"created_at": time.Now().Format(time.RFC3339),
			},
		},
	}

	// Сохраняем события в outbox
	for _, eventData := range events {
		event, err := outbox.NewOutboxEvent(
			eventData.eventID,
			eventData.eventType,
			eventData.aggregateType,
			eventData.aggregateID,
			eventData.topic,
			eventData.payload,
		)
		if err != nil {
			logger.Error("Failed to create outbox event",
				zap.String("event_id", eventData.eventID),
				zap.Error(err))
			continue
		}

		// Сохраняем событие с автоматическим извлечением trace info
		if err := outbox.SaveEventWithTrace(ctx, tx, event); err != nil {
			logger.Error("Failed to save outbox event",
				zap.String("event_id", eventData.eventID),
				zap.Error(err))
			continue
		}

		logger.Info("Event saved to outbox",
			zap.String("event_id", eventData.eventID),
			zap.String("event_type", eventData.eventType),
			zap.String("topic", eventData.topic))
	}

	// Коммитим транзакцию
	if err := tx.Commit(); err != nil {
		logger.Error("Failed to commit transaction", zap.Error(err))
		return
	}

	logger.Info("All events saved to outbox successfully")
}
