package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrEventAlreadyExists = errors.New("event already exists")
)

type Event struct {
	EventID       string      `json:"event_id"`
	EventType     string      `json:"event_type"`
	AggregateType string      `json:"aggregate_type"`
	AggregateID   string      `json:"aggregate_id"`
	Topic         string      `json:"topic"`
	Payload       interface{} `json:"payload"`
	TraceID       string      `json:"trace_id,omitempty"`
	SpanID        string      `json:"span_id,omitempty"`
}

func NewOutboxEvent(eventID, eventType, aggregateType, aggregateID, topic string, payload interface{}) (Event, error) {
	event := Event{
		EventID:       eventID,
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Topic:         topic,
		Payload:       payload,
	}

	if err := validateOutboxEvent(event); err != nil {
		return Event{}, err
	}

	return event, nil
}

func SaveEvent(ctx context.Context, tx *sql.Tx, event Event) error {
	if err := validateOutboxEvent(event); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	eventWithTrace := event
	if event.TraceID == "" || event.SpanID == "" {
		traceID, spanID := extractTraceInfo(ctx)
		if event.TraceID == "" {
			eventWithTrace.TraceID = traceID
		}
		if event.SpanID == "" {
			eventWithTrace.SpanID = spanID
		}
	}

	query := `
		INSERT INTO outbox_events 
		(event_id, event_type, aggregate_type, aggregate_id, topic, payload, trace_id, span_id, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	_, err = tx.ExecContext(ctx, query,
		event.EventID,
		event.EventType,
		event.AggregateType,
		event.AggregateID,
		event.Topic,
		payloadJSON,
		nullString(event.TraceID),
		nullString(event.SpanID),
		EventRecordStatusNew,
	)

	if err != nil {
		return fmt.Errorf("failed to save outbox event: %w", convertFromDBError(err))
	}

	return nil
}

func convertFromDBError(err error) error {
	var msqlError *mysql.MySQLError
	if ok := errors.As(err, &msqlError); ok {
		switch msqlError.Number {
		case 1062: // err duplicate rows
			return ErrEventAlreadyExists
		}
	}

	return err
}

func SaveEventWithTrace(ctx context.Context, tx *sql.Tx, event Event) error {
	traceID, spanID := extractTraceInfo(ctx)
	event.TraceID = traceID
	event.SpanID = spanID

	return SaveEvent(ctx, tx, event)
}

func SaveEventRecord(ctx context.Context, tx *sql.Tx, event EventRecord) error {
	query := `
		INSERT INTO outbox_events 
		(event_id, event_type, aggregate_type, aggregate_id, topic, payload, trace_id, span_id, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := tx.ExecContext(ctx, query,
		event.EventID,
		event.EventType,
		event.AggregateType,
		event.AggregateID,
		event.Topic,
		event.Payload,
		nullString(event.TraceID),
		nullString(event.SpanID),
		EventRecordStatusNew,
	)

	if err != nil {
		return fmt.Errorf("failed to save outbox event: %w", err)
	}

	return nil
}

func ensureOutboxTable(ctx context.Context, db *sql.DB) error {
	err := createOutboxEventsTable(ctx, db)
	if err != nil {
		return err
	}

	err = createOutboxDeadlettersTable(ctx, db)
	if err != nil {
		return err
	}

	return nil
}

func createOutboxEventsTable(ctx context.Context, db *sql.DB) error {
	query := `
		CREATE TABLE IF NOT EXISTS outbox_events (
			id              BIGINT AUTO_INCREMENT PRIMARY KEY,
			event_id        CHAR(36)     NOT NULL UNIQUE,
			event_type      VARCHAR(255) NOT NULL,
			aggregate_type  VARCHAR(255) NOT NULL,
			aggregate_id    VARCHAR(255) NOT NULL,
			status          INT          NOT NULL DEFAULT 0 COMMENT '0 - new, 1 - success, 2 - retry, 3 - error, 4 - processing',
			topic           VARCHAR(255) NOT NULL,
			payload         JSON         NOT NULL,
			trace_id        CHAR(36)     NULL,
			span_id         CHAR(36)     NULL,
			attempt_count   INT          NOT NULL DEFAULT 0,
			next_attempt_at TIMESTAMP    NULL,
			last_error      TEXT         NULL,
			created_at      TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			updated_at      TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
			INDEX idx_status_next_attempt (status, next_attempt_at),
			INDEX idx_aggregate (aggregate_type, aggregate_id),
			INDEX idx_created_at (created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`

	_, err := db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create outbox_events table: %w", err)
	}

	return nil
}

func createOutboxDeadlettersTable(ctx context.Context, db *sql.DB) error {
	query := `
		CREATE TABLE IF NOT EXISTS outbox_deadletters
		(
		    id              BIGINT PRIMARY KEY,
		    event_id        CHAR(36)      NOT NULL UNIQUE,
		    event_type      VARCHAR(255)  NOT NULL,
		    aggregate_type  VARCHAR(255)  NOT NULL,
		    aggregate_id    VARCHAR(255)  NOT NULL,
		    topic           VARCHAR(255)  NOT NULL,
		    payload         JSON          NOT NULL,
		    trace_id        CHAR(36)      NULL,
		    span_id         CHAR(36)      NULL,
		    attempt_count   INT           NOT NULL,
		    last_error      VARCHAR(2000) NULL,
		    created_at      TIMESTAMP(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`

	_, err := db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create outbox_deadletters table: %w", err)
	}

	return nil
}

func extractTraceInfo(ctx context.Context) (traceID, spanID string) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		traceID = span.SpanContext().TraceID().String()
		spanID = span.SpanContext().SpanID().String()
	}
	return traceID, spanID
}

func validateOutboxEvent(event Event) error {
	if event.AggregateType == "" {
		return fmt.Errorf("aggregate_type is required")
	}
	if event.AggregateID == "" {
		return fmt.Errorf("aggregate_id is required")
	}
	if event.Topic == "" {
		return fmt.Errorf("topic is required")
	}
	return nil
}
