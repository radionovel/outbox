# Outbox Pattern Library for Go

[![Go Version](https://img.shields.io/badge/go-1.23.0-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/overtonx/outbox)](https://goreportcard.com/report/github.com/overtonx/outbox)

A robust, production-ready Go library implementing the **Outbox Pattern** for reliable event publishing in distributed systems. This library ensures transactional consistency between your database and message brokers like Kafka.

## 🚀 Features

- **✅ Transactional Consistency** - Events are saved in the same transaction as your business data
- **🔄 Automatic Retry Logic** - Configurable retry strategies with exponential backoff
- **💀 Dead Letter Queue** - Failed events are moved to a dead letter queue for manual inspection
- **🔍 Stuck Event Recovery** - Automatically recovers events stuck in processing state
- **📊 Metrics & Monitoring** - Built-in OpenTelemetry metrics and structured logging
- **🎯 Multiple Publishers** - Support for Kafka and custom publishers
- **⚡ High Performance** - Batch processing and configurable polling intervals
- **🧹 Automatic Cleanup** - Configurable retention policies for old events
- **🔗 Distributed Tracing** - Automatic trace ID extraction and propagation

## 📋 Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [API Reference](#api-reference)
- [Configuration](#configuration)
- [Examples](#examples)
- [Monitoring](#monitoring)
- [Contributing](#contributing)
- [License](#license)

## 🛠 Installation

```bash
go get github.com/overtonx/outbox
```

## 🚀 Quick Start

### 1. Database Setup

```go
import (
    "database/sql"
    _ "github.com/go-sql-driver/mysql"
    "github.com/overtonx/outbox"
)

// Connect to your database
db, err := sql.Open("mysql", "user:pass@tcp(localhost:3306)/dbname?parseTime=true")
if err != nil {
    log.Fatal(err)
}

// Create outbox tables
ctx := context.Background()
if err := outbox.CreateOutboxTable(ctx, db); err != nil {
    log.Fatal(err)
}
```

### 2. Save Events

```go
// Create an event
event, err := outbox.NewOutboxEvent(
    "evt-123",                    // event ID
    "UserCreated",                // event type
    "User",                       // aggregate type
    "user-456",                   // aggregate ID
    "user-events",                // topic
    map[string]interface{}{       // payload
        "user_id": "user-456",
        "email": "john@example.com",
        "name": "John Doe",
    },
)
if err != nil {
    log.Fatal(err)
}

// Save event in transaction
tx, err := db.BeginTx(ctx, nil)
if err != nil {
    log.Fatal(err)
}
defer tx.Rollback()

// Save with automatic trace extraction
if err := outbox.SaveEventWithTrace(ctx, tx, event); err != nil {
    log.Fatal(err)
}

// Commit transaction
if err := tx.Commit(); err != nil {
    log.Fatal(err)
}
```

### 3. Start Dispatcher

```go
// Create dispatcher with Kafka publisher
dispatcher := outbox.NewDispatcher(db,
    outbox.WithKafkaConfig(outbox.KafkaConfig{
        Brokers: []string{"localhost:9092"},
        Topic:   "user-events",
    }),
    outbox.WithLogger(logger),
)

// Start processing events
go dispatcher.Start(ctx)
```

## 🏗 Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Application   │───▶│   Database      │    │     Kafka       │
│                 │    │                 │    │                 │
│ 1. Save Event   │    │ 2. Save to      │    │ 4. Publish      │
│    to Outbox    │    │    outbox_events│    │    Event        │
│                 │    │                 │    │                 │
│ 3. Commit TX    │    │ 3. Commit TX    │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                │
                                ▼
                       ┌─────────────────┐
                       │   Dispatcher    │
                       │                 │
                       │ - Event Processor
                       │ - Dead Letter Service
                       │ - Stuck Event Service
                       │ - Cleanup Service
                       └─────────────────┘
```

### Components

- **Event** - Represents a domain event with metadata
- **Dispatcher** - Main orchestrator managing all workers
- **EventProcessor** - Processes events and publishes to message broker
- **DeadLetterService** - Handles failed events
- **StuckEventService** - Recovers stuck events
- **CleanupService** - Cleans up old events
- **Publisher** - Interface for publishing events (Kafka, custom)

## 📚 API Reference

### Core Types

```go
type Event struct {
    EventID       string                 `json:"event_id"`
    EventType     string                 `json:"event_type"`
    AggregateType string                 `json:"aggregate_type"`
    AggregateID   string                 `json:"aggregate_id"`
    Topic         string                 `json:"topic"`
    Payload       map[string]interface{} `json:"payload"`
    TraceID       string                 `json:"trace_id,omitempty"`
    SpanID        string                 `json:"span_id,omitempty"`
}

type EventRecord struct {
    ID            int64
    AggregateType string
    AggregateID   string
    EventID       string
    EventType     string
    Payload       []byte
    Topic         string
    TraceID       string
    SpanID        string
    AttemptCount  int
    NextAttemptAt *time.Time
}
```

### Main Functions

```go
// Create a new outbox event
func NewOutboxEvent(eventID, eventType, aggregateType, aggregateID, topic string, payload map[string]interface{}) (Event, error)

// Save event in transaction
func SaveEvent(ctx context.Context, tx *sql.Tx, event Event) error

// Save event with automatic trace extraction
func SaveEventWithTrace(ctx context.Context, tx *sql.Tx, event Event) error

// Create outbox tables
func CreateOutboxTable(ctx context.Context, db *sql.DB) error

// Create dispatcher
func NewDispatcher(db *sql.DB, opts ...DispatcherOption) *Dispatcher
```

### Interfaces

```go
type Publisher interface {
    Publish(ctx context.Context, event EventRecord) error
}

type MetricsCollector interface {
    IncrementCounter(name string, tags map[string]string)
    RecordDuration(name string, duration time.Duration, tags map[string]string)
    RecordGauge(name string, value float64, tags map[string]string)
}

type BackoffStrategy interface {
    CalculateNextAttempt(attemptCount int) time.Time
}
```

## ⚙️ Configuration

### Dispatcher Options

```go
dispatcher := outbox.NewDispatcher(db,
    outbox.WithBatchSize(100),                    // Events per batch
    outbox.WithPollInterval(2*time.Second),       // Polling frequency
    outbox.WithMaxAttempts(3),                    // Max retry attempts
    outbox.WithDeadLetterInterval(5*time.Minute), // Dead letter check interval
    outbox.WithStuckEventTimeout(10*time.Minute), // Stuck event timeout
    outbox.WithDeadLetterRetention(7*24*time.Hour), // Dead letter retention
    outbox.WithSentEventsRetention(24*time.Hour),   // Sent events retention
    outbox.WithCleanupInterval(1*time.Hour),        // Cleanup interval
    outbox.WithBackoffStrategy(strategy),           // Retry strategy
    outbox.WithPublisher(publisher),                // Custom publisher
    outbox.WithMetrics(metrics),                    // Metrics collector
    outbox.WithLogger(logger),                      // Logger
)
```

### Kafka Configuration

```go
kafkaConfig := outbox.KafkaConfig{
    Brokers:      []string{"localhost:9092"},
    Topic:        "events",
    BatchSize:    10,
    BatchTimeout: 100 * time.Millisecond,
    Async:        false,
    Compression:  kafka.Snappy,
    RequiredAcks: kafka.RequireAll,
}
```

### Backoff Strategies

```go
// Exponential backoff (default)
strategy := &outbox.ExponentialBackoffStrategy{
    BaseDelay: 1 * time.Minute,
    MaxDelay:  30 * time.Minute,
}

// Linear backoff
strategy := &outbox.LinearBackoffStrategy{
    BaseDelay: 1 * time.Minute,
}

// Fixed delay
strategy := &outbox.FixedBackoffStrategy{
    Delay: 5 * time.Minute,
}
```

## 📖 Examples

### Basic Usage

```go
package main

import (
    "context"
    "database/sql"
    "log"
    
    _ "github.com/go-sql-driver/mysql"
    "github.com/overtonx/outbox"
)

func main() {
    db, _ := sql.Open("mysql", "user:pass@tcp(localhost:3306)/dbname")
    
    // Create tables
    outbox.CreateOutboxTable(context.Background(), db)
    
    // Create dispatcher
    dispatcher := outbox.NewDispatcher(db)
    
    // Start processing
    go dispatcher.Start(context.Background())
    
    // Save events in your business logic
    tx, _ := db.BeginTx(context.Background(), nil)
    defer tx.Rollback()
    
    event, _ := outbox.NewOutboxEvent(
        "evt-123",
        "UserCreated",
        "User",
        "user-456",
        "user-events",
        map[string]interface{}{
            "user_id": "user-456",
            "email": "john@example.com",
        },
    )
    
    outbox.SaveEventWithTrace(context.Background(), tx, event)
    tx.Commit()
}
```

### Custom Publisher

```go
type CustomPublisher struct {
    logger *zap.Logger
}

func (p *CustomPublisher) Publish(ctx context.Context, event outbox.EventRecord) error {
    p.logger.Info("Publishing event",
        zap.String("event_id", event.EventID),
        zap.String("event_type", event.EventType),
    )
    
    // Your custom publishing logic here
    return nil
}

// Use custom publisher
dispatcher := outbox.NewDispatcher(db,
    outbox.WithPublisher(&CustomPublisher{logger: logger}),
)
```

### Custom Metrics

```go
type CustomMetrics struct{}

func (m *CustomMetrics) IncrementCounter(name string, tags map[string]string) {
    // Your metrics implementation
}

func (m *CustomMetrics) RecordDuration(name string, duration time.Duration, tags map[string]string) {
    // Your metrics implementation
}

func (m *CustomMetrics) RecordGauge(name string, value float64, tags map[string]string) {
    // Your metrics implementation
}

// Use custom metrics
dispatcher := outbox.NewDispatcher(db,
    outbox.WithMetrics(&CustomMetrics{}),
)
```

## 📊 Monitoring

### Built-in Metrics

The library provides OpenTelemetry metrics out of the box:

- `outbox.events.processed` - Number of events processed
- `outbox.events.published` - Number of events successfully published
- `outbox.events.failed` - Number of events that failed
- `outbox.events.retried` - Number of events retried
- `outbox.events.dead_lettered` - Number of events moved to dead letter queue
- `outbox.processing.duration` - Processing duration
- `outbox.queue.size` - Current queue size

### Logging

Structured logging with zap:

```go
logger, _ := zap.NewDevelopment()
dispatcher := outbox.NewDispatcher(db,
    outbox.WithLogger(logger),
)
```

### Health Checks

```go
// Check if dispatcher is running
if dispatcher.IsStarted() {
    log.Println("Dispatcher is running")
}

// Get metrics
metrics := dispatcher.GetMetrics()
log.Printf("Metrics: %+v", metrics)
```

## 🗄️ Database Schema

### outbox_events Table

```sql
CREATE TABLE outbox_events (
    id              bigint auto_increment primary key,
    event_id        char(36)     not null unique,
    event_type      varchar(255) not null,
    aggregate_type  varchar(255) not null,
    aggregate_id    varchar(255) not null,
    status          int          not null default 0,
    topic           varchar(255) not null,
    payload         json         not null,
    trace_id        char(36)     null,
    span_id         char(36)     null,
    attempt_count   int          not null default 0,
    next_attempt_at timestamp    null,
    last_error      text         null,
    created_at      timestamp(6) not null default current_timestamp(6),
    updated_at      timestamp(6) not null default current_timestamp(6) on update current_timestamp(6),
    INDEX idx_status_next_attempt (status, next_attempt_at),
    INDEX idx_aggregate (aggregate_type, aggregate_id),
    INDEX idx_created_at (created_at)
);
```

### outbox_deadletters Table

```sql
CREATE TABLE outbox_deadletters (
    id              bigint primary key,
    event_id        char(36)      not null unique,
    event_type      varchar(255)  not null,
    aggregate_type  varchar(255)  not null,
    aggregate_id    varchar(255)  not null,
    topic           varchar(255)  not null,
    payload         json          not null,
    trace_id        char(36)      null,
    span_id         char(36)      null,
    attempt_count   int           not null,
    last_error      varchar(2000) null,
    created_at      timestamp(6)  not null default current_timestamp(6)
);
```

## 🔧 Advanced Usage

### Event Statuses

- `0` - New (waiting to be processed)
- `1` - Sent (successfully published)
- `2` - Retry (scheduled for retry)
- `3` - Error (moved to dead letter queue)
- `4` - Processing (currently being processed)

### Error Handling

The library handles various error scenarios:

1. **Network failures** - Events are retried with exponential backoff
2. **Broker unavailable** - Events remain in retry status
3. **Serialization errors** - Events are moved to dead letter queue
4. **Stuck events** - Automatically recovered and retried

### Performance Tuning

```go
// High throughput configuration
dispatcher := outbox.NewDispatcher(db,
    outbox.WithBatchSize(1000),           // Larger batches
    outbox.WithPollInterval(100*time.Millisecond), // Faster polling
    outbox.WithKafkaConfig(outbox.KafkaConfig{
        BatchSize:    100,                // Kafka batch size
        BatchTimeout: 10 * time.Millisecond,
        Async:        true,               // Async publishing
    }),
)
```

## 🧪 Testing

```bash
# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific test
go test -run TestDispatcher ./...
```

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📞 Support

- 📧 Email: support@overtonx.com
- 🐛 Issues: [GitHub Issues](https://github.com/overtonx/outbox/issues)
- 📖 Documentation: [GitHub Wiki](https://github.com/overtonx/outbox/wiki)

## 🙏 Acknowledgments

- Inspired by the Outbox Pattern described in microservices patterns
- Built with [Kafka Go](https://github.com/segmentio/kafka-go)
- Uses [OpenTelemetry](https://opentelemetry.io/) for observability
- Structured logging with [Zap](https://github.com/uber-go/zap)
