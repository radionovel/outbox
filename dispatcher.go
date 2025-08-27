package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// DispatcherConfig содержит конфигурацию диспетчера
type DispatcherConfig struct {
	BatchSize       int           `json:"batch_size"`
	PollInterval    time.Duration `json:"poll_interval"`
	MaxAttempts     int           `json:"max_attempts"`
	BackoffStrategy BackoffStrategy `json:"-"`
}

// DefaultDispatcherConfig возвращает стандартную конфигурацию диспетчера
func DefaultDispatcherConfig() DispatcherConfig {
	return DispatcherConfig{
		BatchSize:       100,
		PollInterval:    1 * time.Second,
		MaxAttempts:     5,
		BackoffStrategy: DefaultBackoffStrategy(),
	}
}

// Dispatcher представляет воркер для обработки событий из outbox
type Dispatcher struct {
	config     DispatcherConfig
	db         *sql.DB
	producer   *KafkaProducer
	stopChan   chan struct{}
	wg         sync.WaitGroup
	mu         sync.RWMutex
	isRunning  bool
	logger     *log.Logger
}

// NewDispatcher создает новый диспетчер
func NewDispatcher(config DispatcherConfig, db *sql.DB, producer *KafkaProducer) *Dispatcher {
	if config.BackoffStrategy == nil {
		config.BackoffStrategy = DefaultBackoffStrategy()
	}

	return &Dispatcher{
		config:   config,
		db:       db,
		producer: producer,
		stopChan: make(chan struct{}),
		logger:   log.Default(),
	}
}

// Start запускает диспетчер в отдельной горутине
func (d *Dispatcher) Start(ctx context.Context) {
	d.mu.Lock()
	if d.isRunning {
		d.mu.Unlock()
		return
	}
	d.isRunning = true
	d.mu.Unlock()

	d.wg.Add(1)
	go d.run(ctx)
}

// Stop останавливает диспетчер
func (d *Dispatcher) Stop() {
	d.mu.Lock()
	if !d.isRunning {
		d.mu.Unlock()
		return
	}
	d.isRunning = false
	d.mu.Unlock()

	close(d.stopChan)
	d.wg.Wait()
}

// IsRunning возвращает true, если диспетчер запущен
func (d *Dispatcher) IsRunning() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.isRunning
}

// run основная логика диспетчера
func (d *Dispatcher) run(ctx context.Context) {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Println("Dispatcher context cancelled")
			return
		case <-d.stopChan:
			d.logger.Println("Dispatcher stopped")
			return
		case <-ticker.C:
			if err := d.processBatch(ctx); err != nil {
				d.logger.Printf("Error processing batch: %v", err)
			}
		}
	}
}

// processBatch обрабатывает пакет событий
func (d *Dispatcher) processBatch(ctx context.Context) error {
	events, err := d.fetchPendingEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch pending events: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	d.logger.Printf("Processing %d events", len(events))

	for _, event := range events {
		if err := d.processEvent(ctx, event); err != nil {
			d.logger.Printf("Failed to process event %s: %v", event.EventID, err)
		}
	}

	return nil
}

// fetchPendingEvents получает события для обработки с блокировкой FOR UPDATE SKIP LOCKED
func (d *Dispatcher) fetchPendingEvents(ctx context.Context) ([]OutboxRecord, error) {
	query := `
		SELECT id, event_id, aggregate_type, aggregate_id, event_type, 
		       payload, status, attempts, next_attempt_at, created_at, last_error
		FROM outbox 
		WHERE (status = 'pending' OR (status = 'failed' AND next_attempt_at <= NOW()))
		ORDER BY created_at ASC
		LIMIT ?
		FOR UPDATE SKIP LOCKED
	`

	rows, err := d.db.QueryContext(ctx, query, d.config.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending events: %w", err)
	}
	defer rows.Close()

	var events []OutboxRecord
	for rows.Next() {
		var event OutboxRecord
		var nextAttemptAt sql.NullTime
		var lastError sql.NullString

		err := rows.Scan(
			&event.ID,
			&event.EventID,
			&event.AggregateType,
			&event.AggregateID,
			&event.EventType,
			&event.Payload,
			&event.Status,
			&event.Attempts,
			&nextAttemptAt,
			&event.CreatedAt,
			&lastError,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		if nextAttemptAt.Valid {
			event.NextAttemptAt = &nextAttemptAt.Time
		}
		if lastError.Valid {
			event.LastError = &lastError.String
		}

		events = append(events, event)
	}

	return events, nil
}

// processEvent обрабатывает одно событие
func (d *Dispatcher) processEvent(ctx context.Context, event OutboxRecord) error {
	// Обновляем статус на "sending"
	if err := d.updateEventStatus(ctx, event.ID, StatusSending, event.Attempts, nil, nil); err != nil {
		return fmt.Errorf("failed to update status to sending: %w", err)
	}

	// Десериализуем payload
	var payload map[string]interface{}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Создаем OutboxEvent для отправки
	outboxEvent := OutboxEvent{
		EventID:       event.EventID,
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		EventType:     event.EventType,
		Payload:       payload,
	}

	// Отправляем в Kafka
	if err := d.producer.PublishEvent(ctx, outboxEvent); err != nil {
		// Обрабатываем ошибку
		return d.handlePublishError(ctx, event, err)
	}

	// Успешная отправка
	return d.updateEventStatus(ctx, event.ID, StatusSent, event.Attempts, nil, nil)
}

// handlePublishError обрабатывает ошибку публикации
func (d *Dispatcher) handlePublishError(ctx context.Context, event OutboxRecord, publishErr error) error {
	newAttempts := event.Attempts + 1
	errorMsg := publishErr.Error()

	if newAttempts >= d.config.MaxAttempts {
		// Достигнут лимит попыток
		return d.updateEventStatus(ctx, event.ID, StatusFailed, newAttempts, nil, &errorMsg)
	}

	// Вычисляем время следующей попытки
	nextAttemptAt := time.Now().Add(d.config.BackoffStrategy(newAttempts))
	
	return d.updateEventStatus(ctx, event.ID, StatusFailed, newAttempts, &nextAttemptAt, &errorMsg)
}

// updateEventStatus обновляет статус события в базе
func (d *Dispatcher) updateEventStatus(ctx context.Context, id int64, status EventStatus, attempts int, nextAttemptAt *time.Time, lastError *string) error {
	var query string
	var args []interface{}

	if nextAttemptAt != nil && lastError != nil {
		query = `
			UPDATE outbox 
			SET status = ?, attempts = ?, next_attempt_at = ?, last_error = ?
			WHERE id = ?
		`
		args = []interface{}{status, attempts, nextAttemptAt, lastError, id}
	} else if nextAttemptAt != nil {
		query = `
			UPDATE outbox 
			SET status = ?, attempts = ?, next_attempt_at = ?
			WHERE id = ?
		`
		args = []interface{}{status, attempts, nextAttemptAt, id}
	} else if lastError != nil {
		query = `
			UPDATE outbox 
			SET status = ?, attempts = ?, last_error = ?
			WHERE id = ?
		`
		args = []interface{}{status, attempts, lastError, id}
	} else {
		query = `
			UPDATE outbox 
			SET status = ?, attempts = ?
			WHERE id = ?
		`
		args = []interface{}{status, attempts, id}
	}

	_, err := d.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update event status: %w", err)
	}

	return nil
}

// GetStats возвращает статистику диспетчера
func (d *Dispatcher) GetStats(ctx context.Context) (map[string]int, error) {
	query := `
		SELECT status, COUNT(*) as count
		FROM outbox
		GROUP BY status
	`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan stats: %w", err)
		}
		stats[status] = count
	}

	return stats, nil
}
