package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"outbox"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// Конфигурация подключений
	mysqlDSN := "root:password@tcp(localhost:3306)/outbox_db?parseTime=true&multiStatements=true"
	kafkaBrokers := []string{"localhost:9092"}
	kafkaTopic := "outbox_events"

	// Подключение к MySQL
	db, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		log.Fatalf("Failed to connect to MySQL: %v", err)
	}
	defer db.Close()

	// Проверяем соединение
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping MySQL: %v", err)
	}

	// Создаем таблицу outbox
	ctx := context.Background()
	if err := outbox.CreateOutboxTable(ctx, db); err != nil {
		log.Fatalf("Failed to create outbox table: %v", err)
	}
	log.Println("Outbox table created successfully")

	// Создаем Kafka продюсер
	producer := outbox.NewKafkaProducer(kafkaBrokers, kafkaTopic)
	defer producer.Close()

	// Проверяем доступность Kafka
	if err := producer.HealthCheck(ctx); err != nil {
		log.Printf("Warning: Kafka health check failed: %v", err)
	} else {
		log.Println("Kafka connection established")
	}

	// Создаем и настраиваем диспетчер
	config := outbox.DispatcherConfig{
		BatchSize:       50,
		PollInterval:    2 * time.Second,
		MaxAttempts:     3,
		BackoffStrategy: outbox.DefaultBackoffStrategy(),
	}

	dispatcher := outbox.NewDispatcher(config, db, producer)

	// Запускаем диспетчер
	dispatcher.Start(ctx)
	log.Println("Dispatcher started")

	// Симуляция бизнес-логики: создание пользователя с событием
	go simulateUserCreation(ctx, db)

	// Ожидаем сигнала для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Периодически выводим статистику
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if stats, err := dispatcher.GetStats(ctx); err == nil {
					log.Printf("Outbox stats: %+v", stats)
				}
			}
		}
	}()

	<-sigChan
	log.Println("Shutting down...")

	// Останавливаем диспетчер
	dispatcher.Stop()
	log.Println("Dispatcher stopped")
}

// simulateUserCreation симулирует создание пользователя с сохранением события в outbox
func simulateUserCreation(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	userID := 1
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := createUserWithEvent(ctx, db, userID); err != nil {
				log.Printf("Failed to create user with event: %v", err)
			} else {
				log.Printf("User %d created with outbox event", userID)
				userID++
			}
		}
	}
}

// createUserWithEvent создает пользователя и сохраняет событие в outbox в одной транзакции
func createUserWithEvent(ctx context.Context, db *sql.DB, userID int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Создаем пользователя (симуляция бизнес-логики)
	userQuery := `INSERT INTO users (id, name, email, created_at) VALUES (?, ?, ?, ?)`
	userName := fmt.Sprintf("User%d", userID)
	userEmail := fmt.Sprintf("user%d@example.com", userID)
	
	_, err = tx.ExecContext(ctx, userQuery, userID, userName, userEmail, time.Now())
	if err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}

	// Создаем событие для outbox
	event := outbox.OutboxEvent{
		EventID:       fmt.Sprintf("user-created-%d-%d", userID, time.Now().Unix()),
		AggregateType: "user",
		AggregateID:   fmt.Sprintf("%d", userID),
		EventType:     "user.created",
		Payload: map[string]interface{}{
			"user_id":   userID,
			"name":      userName,
			"email":     userEmail,
			"timestamp": time.Now().Unix(),
		},
	}

	// Сохраняем событие в outbox
	if err := outbox.SaveEvent(ctx, tx, event); err != nil {
		return fmt.Errorf("failed to save outbox event: %v", err)
	}

	// Коммитим транзакцию
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

// createUsersTable создает таблицу пользователей для демонстрации
func createUsersTable(ctx context.Context, db *sql.DB) error {
	query := `
		CREATE TABLE IF NOT EXISTS users (
			id INT PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) NOT NULL,
			created_at DATETIME NOT NULL,
			INDEX idx_email (email)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`

	_, err := db.ExecContext(ctx, query)
	return err
}
