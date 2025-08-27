-- Инициализация базы данных для outbox pattern
-- Этот файл выполняется автоматически при запуске MySQL контейнера

-- Создание базы данных (если не существует)
CREATE DATABASE IF NOT EXISTS outbox_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Использование базы данных
USE outbox_db;

-- Создание таблицы outbox
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
    INDEX idx_created_at (created_at),
    INDEX idx_aggregate (aggregate_type, aggregate_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Создание таблицы пользователей для демонстрации
CREATE TABLE IF NOT EXISTS users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    created_at DATETIME DEFAULT NOW(),
    updated_at DATETIME DEFAULT NOW() ON UPDATE NOW(),
    INDEX idx_email (email),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Создание таблицы заказов для демонстрации
CREATE TABLE IF NOT EXISTS orders (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    total_amount DECIMAL(10,2) NOT NULL,
    status ENUM('pending','confirmed','shipped','delivered','cancelled') DEFAULT 'pending',
    created_at DATETIME DEFAULT NOW(),
    updated_at DATETIME DEFAULT NOW() ON UPDATE NOW(),
    FOREIGN KEY (user_id) REFERENCES users(id),
    INDEX idx_user_id (user_id),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Создание пользователя для приложения
CREATE USER IF NOT EXISTS 'outbox_user'@'%' IDENTIFIED BY 'outbox_pass';
GRANT ALL PRIVILEGES ON outbox_db.* TO 'outbox_user'@'%';
FLUSH PRIVILEGES;

-- Вставка тестовых данных (опционально)
INSERT IGNORE INTO users (name, email) VALUES 
('Test User 1', 'user1@example.com'),
('Test User 2', 'user2@example.com'),
('Test User 3', 'user3@example.com');

-- Создание представления для мониторинга
CREATE OR REPLACE VIEW outbox_stats AS
SELECT 
    status,
    COUNT(*) as count,
    MIN(created_at) as oldest_event,
    MAX(created_at) as newest_event,
    AVG(attempts) as avg_attempts
FROM outbox 
GROUP BY status;

-- Создание представления для событий с ошибками
CREATE OR REPLACE VIEW outbox_errors AS
SELECT 
    id,
    event_id,
    aggregate_type,
    aggregate_id,
    event_type,
    attempts,
    last_error,
    created_at,
    next_attempt_at
FROM outbox 
WHERE status = 'failed' 
ORDER BY next_attempt_at ASC;
