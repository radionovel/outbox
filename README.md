# Outbox Pattern Package for Go

Пакет `outbox` реализует паттерн Outbox для надежной доставки событий с использованием MySQL и Kafka. Этот паттерн гарантирует, что бизнес-события будут доставлены хотя бы один раз, даже при сбоях в системе.

## Архитектура

Паттерн Outbox работает следующим образом:

1. **Бизнес-транзакция**: При выполнении бизнес-операции (например, создание пользователя) событие сохраняется в таблицу `outbox` в той же транзакции, что и бизнес-данные.
2. **Асинхронная обработка**: Отдельный воркер (Dispatcher) периодически опрашивает таблицу `outbox` и отправляет события в Kafka.
3. **Гарантия доставки**: Если Kafka недоступна, события остаются в таблице и будут повторно обработаны позже.

## Установка

```bash
go get github.com/your-username/outbox
```

## Зависимости

- Go 1.21+
- MySQL 8.0+
- Kafka 2.8+
- Docker (для тестов)

## Структура таблицы outbox

```sql
CREATE TABLE outbox (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    event_id CHAR(36) UNIQUE NOT NULL,
    aggregate_type VARCHAR(255) NOT NULL,
    aggregate_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(255) NOT NULL,
    payload JSON NOT NULL,
    status ENUM('pending','sending','sent','failed') DEFAULT 'pending',
    attempts INT DEFAULT 0,
    next_attempt_at DATETIME DEFAULT NOW(),
    created_at DATETIME DEFAULT NOW(),
    last_error TEXT NULL,
    INDEX idx_status_next_attempt (status, next_attempt_at),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

## Основные компоненты

### 1. OutboxEvent

Структура для представления события:

```go
type OutboxEvent struct {
    EventID       string                 `json:"event_id"`
    AggregateType string                 `json:"aggregate_type"`
    AggregateID   string                 `json:"aggregate_id"`
    EventType     string                 `json:"event_type"`
    Payload       map[string]interface{} `json:"payload"`
}
```

### 2. SaveEvent

Функция для сохранения события в outbox в рамках транзакции:

```go
func SaveEvent(ctx context.Context, tx *sql.Tx, event OutboxEvent) error
```

### 3. Dispatcher

Воркер для обработки событий из outbox и отправки их в Kafka:

```go
type Dispatcher struct {
    // ... внутренние поля
}

func NewDispatcher(config DispatcherConfig, db *sql.DB, producer *KafkaProducer) *Dispatcher
func (d *Dispatcher) Start(ctx context.Context)
func (d *Dispatcher) Stop()
func (d *Dispatcher) IsRunning() bool
```

### 4. KafkaProducer

Обёртка для Kafka продюсера с настройками идемпотентности:

```go
type KafkaProducer struct {
    // ... внутренние поля
}

func NewKafkaProducer(brokers []string, topic string) *KafkaProducer
func (p *KafkaProducer) PublishEvent(ctx context.Context, event OutboxEvent) error
func (p *KafkaProducer) PublishEventsBatch(ctx context.Context, events []OutboxEvent) error
func (p *KafkaProducer) Close() error
func (p *KafkaProducer) HealthCheck(ctx context.Context) error
```

### 5. BackoffStrategy

Стратегии для вычисления задержки между повторными попытками:

```go
type BackoffStrategy func(attempt int) time.Duration

func ExponentialBackoffWithJitter(baseDelay, maxDelay time.Duration) BackoffStrategy
func LinearBackoffWithJitter(baseDelay, maxDelay time.Duration) BackoffStrategy
func DefaultBackoffStrategy() BackoffStrategy
```

## Пример использования

### Базовое использование

```go
package main

import (
    "context"
    "database/sql"
    "log"
    "time"
    
    "outbox"
    _ "github.com/go-sql-driver/mysql"
)

func main() {
    // Подключение к MySQL
    db, err := sql.Open("mysql", "user:password@tcp(localhost:3306)/dbname")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Создание таблицы outbox
    ctx := context.Background()
    if err := outbox.CreateOutboxTable(ctx, db); err != nil {
        log.Fatal(err)
    }

    // Создание Kafka продюсера
    producer := outbox.NewKafkaProducer([]string{"localhost:9092"}, "events")
    defer producer.Close()

    // Создание и запуск диспетчера
    config := outbox.DefaultDispatcherConfig()
    dispatcher := outbox.NewDispatcher(config, db, producer)
    
    dispatcher.Start(ctx)
    defer dispatcher.Stop()

    // Бизнес-логика с сохранением события
    if err := createUserWithEvent(ctx, db); err != nil {
        log.Fatal(err)
    }

    // Ожидание завершения
    select {}
}

func createUserWithEvent(ctx context.Context, db *sql.DB) error {
    tx, err := db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // Создание пользователя
    _, err = tx.ExecContext(ctx, "INSERT INTO users (name, email) VALUES (?, ?)", "John", "john@example.com")
    if err != nil {
        return err
    }

    // Сохранение события в outbox
    event := outbox.OutboxEvent{
        EventID:       "user-created-123",
        AggregateType: "user",
        AggregateID:   "123",
        EventType:     "user.created",
        Payload: map[string]interface{}{
            "user_id": 123,
            "name":    "John",
            "email":   "john@example.com",
        },
    }

    if err := outbox.SaveEvent(ctx, tx, event); err != nil {
        return err
    }

    return tx.Commit()
}
```

### Настройка диспетчера

```go
config := outbox.DispatcherConfig{
    BatchSize:       100,                    // Размер пакета для обработки
    PollInterval:    1 * time.Second,        // Интервал опроса
    MaxAttempts:     5,                      // Максимальное количество попыток
    BackoffStrategy: outbox.DefaultBackoffStrategy(),
}

// Кастомная стратегия бэкоффа
customBackoff := outbox.ExponentialBackoffWithJitter(
    100*time.Millisecond,  // базовая задержка
    30*time.Second,        // максимальная задержка
)
config.BackoffStrategy = customBackoff
```

## Конфигурация

### MySQL

```sql
-- Создание базы данных
CREATE DATABASE outbox_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Создание пользователя
CREATE USER 'outbox_user'@'%' IDENTIFIED BY 'password';
GRANT ALL PRIVILEGES ON outbox_db.* TO 'outbox_user'@'%';
FLUSH PRIVILEGES;
```

### Kafka

```yaml
# docker-compose.yml
version: '3.8'
services:
  kafka:
    image: confluentinc/cp-kafka:7.4.0
    environment:
      KAFKA_CFG_NODE_ID: 0
      KAFKA_CFG_PROCESS_ROLES: 'broker,controller'
      KAFKA_CFG_LISTENERS: 'PLAINTEXT://:9092,CONTROLLER://:9093'
      KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP: 'CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT'
      KAFKA_CFG_CONTROLLER_LISTENER_NAMES: 'CONTROLLER'
      KAFKA_CFG_INTER_BROKER_LISTENER_NAME: 'PLAINTEXT'
      KAFKA_CFG_ADVERTISED_LISTENERS: 'PLAINTEXT://localhost:9092'
      KAFKA_CFG_CONTROLLER_QUORUM_VOTERS: '0@kafka:9093'
      KAFKA_CFG_LOG_DIRS: '/var/lib/kafka/data'
      KAFKA_CFG_LOG_RETENTION_HOURS: '168'
      KAFKA_CFG_LOG_SEGMENT_BYTES: '1073741824'
      KAFKA_CFG_LOG_RETENTION_CHECK_INTERVAL_MS: '300000'
    ports:
      - "9092:9092"
    volumes:
      - kafka-data:/var/lib/kafka/data

volumes:
  kafka-data:
```

## Тестирование

### Запуск юнит-тестов

```bash
go test ./...
```

### Запуск интеграционных тестов

```bash
# Требуется Docker
go test -tags=integration ./...
```

### Запуск примера

```bash
cd example
go run main.go
```

## Особенности реализации

### 1. Идемпотентность

- Kafka продюсер настроен с `RequiredAcks: RequireAll`
- События отправляются с ключом `aggregate_id` для гарантии порядка
- Потребители должны быть идемпотентными

### 2. Надежность

- Использование `FOR UPDATE SKIP LOCKED` для избежания блокировок
- Экспоненциальный бэкофф с джиттером для повторных попыток
- Логирование всех ошибок и попыток

### 3. Производительность

- Пакетная обработка событий
- Настраиваемые интервалы опроса
- Сжатие сообщений Kafka (Snappy)

### 4. Мониторинг

- Статистика по статусам событий
- Health check для Kafka
- Логирование всех операций

## Ограничения

1. **Порядок событий**: Гарантируется только в рамках одного `aggregate_id`
2. **Дублирование**: Возможны дубли при сбоях (потребители должны быть идемпотентными)
3. **Размер сообщений**: Ограничен настройками Kafka
4. **Время доставки**: Зависит от настроек диспетчера и доступности Kafka

## Лучшие практики

1. **Идентификаторы событий**: Используйте UUID или другие уникальные идентификаторы
2. **Размер payload**: Ограничивайте размер данных в payload
3. **Мониторинг**: Отслеживайте статистику и ошибки
4. **Настройка**: Адаптируйте параметры под вашу нагрузку
5. **Тестирование**: Всегда тестируйте сценарии сбоев

## Лицензия

MIT License
