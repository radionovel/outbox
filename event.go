package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// EventStatus представляет статус события в outbox
type EventStatus string

const (
	StatusPending EventStatus = "pending"
	StatusSending EventStatus = "sending"
	StatusSent   EventStatus = "sent"
	StatusFailed EventStatus = "failed"
)

// OutboxEvent представляет событие для сохранения в outbox
type OutboxEvent struct {
	EventID       string                 `json:"event_id"`
	AggregateType string                 `json:"aggregate_type"`
	AggregateID   string                 `json:"aggregate_id"`
	EventType     string                 `json:"event_type"`
	Payload       map[string]interface{} `json:"payload"`
}

// OutboxRecord представляет запись в таблице outbox
type OutboxRecord struct {
	ID            int64       `db:"id"`
	EventID       string      `db:"event_id"`
	AggregateType string      `db:"aggregate_type"`
	AggregateID   string      `db:"aggregate_id"`
	EventType     string      `db:"event_type"`
	Payload       []byte      `db:"payload"`
	Status        EventStatus `db:"status"`
	Attempts      int         `db:"attempts"`
	NextAttemptAt *time.Time  `db:"next_attempt_at"`
	CreatedAt     time.Time   `db:"created_at"`
	LastError     *string     `db:"last_error"`
}

// SaveEvent сохраняет событие в таблицу outbox в рамках переданной транзакции
func SaveEvent(ctx context.Context, tx *sql.Tx, event OutboxEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	query := `
		INSERT INTO outbox (
			event_id, aggregate_type, aggregate_id, event_type, 
			payload, status, attempts, next_attempt_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	_, err = tx.ExecContext(ctx, query,
		event.EventID,
		event.AggregateType,
		event.AggregateID,
		event.EventType,
		payload,
		StatusPending,
		0,
		now,
		now,
	)

	if err != nil {
		return fmt.Errorf("failed to insert event into outbox: %w", err)
	}

	return nil
}

// CreateOutboxTable создает таблицу outbox с необходимыми полями и индексами
func CreateOutboxTable(ctx context.Context, db *sql.DB) error {
	query := `
		CREATE TABLE IF NOT EXISTS outbox (
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
	`

	_, err := db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create outbox table: %w", err)
	}

	return nil
}
