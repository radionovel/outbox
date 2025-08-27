//go:build integration
// +build integration

package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
	_ "github.com/go-sql-driver/mysql"
)

type testEnvironment struct {
	mysqlContainer *mysql.MySQLContainer
	kafkaContainer *kafka.KafkaContainer
	mysqlDB        *sql.DB
	kafkaProducer  *KafkaProducer
	ctx            context.Context
}

func setupTestEnvironment(t *testing.T) *testEnvironment {
	ctx := context.Background()

	// Запускаем MySQL контейнер
	mysqlContainer, err := mysql.RunContainer(ctx,
		testcontainers.WithImage("mysql:8.0"),
		mysql.WithDatabase("outbox_test"),
		mysql.WithUsername("test"),
		mysql.WithPassword("test"),
		mysql.WithWaitStrategy(wait.ForLog("ready for connections")),
	)
	require.NoError(t, err)

	// Получаем строку подключения к MySQL
	mysqlDSN, err := mysqlContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Подключаемся к MySQL
	mysqlDB, err := sql.Open("mysql", mysqlDSN)
	require.NoError(t, err)
	require.NoError(t, mysqlDB.Ping())

	// Запускаем Kafka контейнер
	kafkaContainer, err := kafka.RunContainer(ctx,
		testcontainers.WithImage("confluentinc/cp-kafka:7.4.0"),
		kafka.WithClusterID("test-cluster"),
		kafka.WithTopics("outbox_events"),
		kafka.WithWaitStrategy(wait.ForLog("started (kafka.server.KafkaServer)")),
	)
	require.NoError(t, err)

	// Получаем адреса брокеров Kafka
	kafkaBrokers, err := kafkaContainer.Brokers(ctx)
	require.NoError(t, err)

	// Создаем Kafka продюсер
	kafkaProducer := NewKafkaProducer(kafkaBrokers, "outbox_events")

	return &testEnvironment{
		mysqlContainer: mysqlContainer,
		kafkaContainer: kafkaContainer,
		mysqlDB:        mysqlDB,
		kafkaProducer:  kafkaProducer,
		ctx:            ctx,
	}
}

func (env *testEnvironment) cleanup() {
	if env.mysqlDB != nil {
		env.mysqlDB.Close()
	}
	if env.kafkaProducer != nil {
		env.kafkaProducer.Close()
	}
	if env.mysqlContainer != nil {
		env.mysqlContainer.Terminate(env.ctx)
	}
	if env.kafkaContainer != nil {
		env.kafkaContainer.Terminate(env.ctx)
	}
}

func TestIntegrationOutboxPattern(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.cleanup()

	// Создаем таблицу outbox
	err := CreateOutboxTable(env.ctx, env.mysqlDB)
	require.NoError(t, err)

	// Создаем тестовую таблицу для бизнес-логики
	_, err = env.mysqlDB.ExecContext(env.ctx, `
		CREATE TABLE test_entities (
			id INT PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			created_at DATETIME NOT NULL
		)
	`)
	require.NoError(t, err)

	t.Run("SaveEvent in transaction", func(t *testing.T) {
		// Начинаем транзакцию
		tx, err := env.mysqlDB.BeginTx(env.ctx, nil)
		require.NoError(t, err)
		defer tx.Rollback()

		// Создаем бизнес-сущность
		_, err = tx.ExecContext(env.ctx, 
			"INSERT INTO test_entities (id, name, created_at) VALUES (?, ?, ?)",
			1, "Test Entity", time.Now())
		require.NoError(t, err)

		// Сохраняем событие в outbox
		event := OutboxEvent{
			EventID:       "test-event-1",
			AggregateType: "test_entity",
			AggregateID:   "1",
			EventType:     "entity.created",
			Payload: map[string]interface{}{
				"entity_id": 1,
				"name":      "Test Entity",
			},
		}

		err = SaveEvent(env.ctx, tx, event)
		require.NoError(t, err)

		// Коммитим транзакцию
		err = tx.Commit()
		require.NoError(t, err)

		// Проверяем, что событие сохранено
		var count int
		err = env.mysqlDB.QueryRowContext(env.ctx, 
			"SELECT COUNT(*) FROM outbox WHERE event_id = ?", "test-event-1").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("Dispatcher processes events", func(t *testing.T) {
		// Создаем несколько событий
		events := []OutboxEvent{
			{
				EventID:       "test-event-2",
				AggregateType: "test_entity",
				AggregateID:   "2",
				EventType:     "entity.created",
				Payload: map[string]interface{}{
					"entity_id": 2,
					"name":      "Test Entity 2",
				},
			},
			{
				EventID:       "test-event-3",
				AggregateType: "test_entity",
				AggregateID:   "3",
				EventType:     "entity.created",
				Payload: map[string]interface{}{
					"entity_id": 3,
					"name":      "Test Entity 3",
				},
			},
		}

		// Сохраняем события
		for _, event := range events {
			_, err := env.mysqlDB.ExecContext(env.ctx, `
				INSERT INTO outbox (
					event_id, aggregate_type, aggregate_id, event_type, 
					payload, status, attempts, next_attempt_at, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, event.EventID, event.AggregateType, event.AggregateID, event.EventType,
				mustMarshalJSON(t, event.Payload), StatusPending, 0, time.Now(), time.Now())
			require.NoError(t, err)
		}

		// Создаем и запускаем диспетчер
		config := DispatcherConfig{
			BatchSize:       10,
			PollInterval:    100 * time.Millisecond,
			MaxAttempts:     3,
			BackoffStrategy: DefaultBackoffStrategy(),
		}

		dispatcher := NewDispatcher(config, env.mysqlDB, env.kafkaProducer)
		
		// Запускаем диспетчер
		dispatcher.Start(env.ctx)
		defer dispatcher.Stop()

		// Ждем обработки событий
		time.Sleep(500 * time.Millisecond)

		// Проверяем, что события обработаны
		var pendingCount, sentCount int
		err = env.mysqlDB.QueryRowContext(env.ctx, 
			"SELECT COUNT(*) FROM outbox WHERE status = ?", StatusPending).Scan(&pendingCount)
		require.NoError(t, err)

		err = env.mysqlDB.QueryRowContext(env.ctx, 
			"SELECT COUNT(*) FROM outbox WHERE status = ?", StatusSent).Scan(&sentCount)
		require.NoError(t, err)

		// Все события должны быть обработаны (либо sent, либо failed)
		assert.Equal(t, 0, pendingCount)
		assert.True(t, sentCount >= 0)
	})

	t.Run("Retry mechanism on failure", func(t *testing.T) {
		// Создаем событие с неверным payload для симуляции ошибки
		_, err := env.mysqlDB.ExecContext(env.ctx, `
			INSERT INTO outbox (
				event_id, aggregate_type, aggregate_id, event_type, 
				payload, status, attempts, next_attempt_at, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, "test-event-4", "test_entity", "4", "entity.created",
			"invalid json", StatusPending, 0, time.Now(), time.Now())
		require.NoError(t, err)

		// Создаем диспетчер с коротким интервалом
		config := DispatcherConfig{
			BatchSize:       10,
			PollInterval:    50 * time.Millisecond,
			MaxAttempts:     2,
			BackoffStrategy: DefaultBackoffStrategy(),
		}

		dispatcher := NewDispatcher(config, env.mysqlDB, env.kafkaProducer)
		dispatcher.Start(env.ctx)
		defer dispatcher.Stop()

		// Ждем обработки
		time.Sleep(300 * time.Millisecond)

		// Проверяем, что событие перешло в статус failed после MaxAttempts
		var status string
		var attempts int
		err = env.mysqlDB.QueryRowContext(env.ctx, 
			"SELECT status, attempts FROM outbox WHERE event_id = ?", "test-event-4").
			Scan(&status, &attempts)
		require.NoError(t, err)

		assert.Equal(t, string(StatusFailed), status)
		assert.Equal(t, 2, attempts)
	})
}

func TestKafkaProducerIntegration(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.cleanup()

	t.Run("PublishEvent to Kafka", func(t *testing.T) {
		event := OutboxEvent{
			EventID:       "kafka-test-1",
			AggregateType: "test_entity",
			AggregateID:   "kafka-1",
			EventType:     "entity.created",
			Payload: map[string]interface{}{
				"entity_id": "kafka-1",
				"name":      "Kafka Test Entity",
			},
		}

		// Отправляем событие в Kafka
		err := env.kafkaProducer.PublishEvent(env.ctx, event)
		require.NoError(t, err)

		// Проверяем health check
		err = env.kafkaProducer.HealthCheck(env.ctx)
		require.NoError(t, err)
	})

	t.Run("PublishEventsBatch to Kafka", func(t *testing.T) {
		events := []OutboxEvent{
			{
				EventID:       "kafka-batch-1",
				AggregateType: "test_entity",
				AggregateID:   "batch-1",
				EventType:     "entity.created",
				Payload: map[string]interface{}{
					"entity_id": "batch-1",
					"name":      "Batch Entity 1",
				},
			},
			{
				EventID:       "kafka-batch-2",
				AggregateType: "test_entity",
				AggregateID:   "batch-2",
				EventType:     "entity.created",
				Payload: map[string]interface{}{
					"entity_id": "batch-2",
					"name":      "Batch Entity 2",
				},
			},
		}

		// Отправляем пакет событий
		err := env.kafkaProducer.PublishEventsBatch(env.ctx, events)
		require.NoError(t, err)
	})
}

// Вспомогательные функции
func mustMarshalJSON(t *testing.T, v interface{}) []byte {
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
