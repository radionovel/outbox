.PHONY: help build test test-unit test-integration run-example clean docker-up docker-down docker-logs

# Переменные
BINARY_NAME=outbox
MAIN_PATH=./example/main.go
DOCKER_COMPOSE_FILE=docker-compose.yml

# Цвета для вывода
GREEN=\033[0;32m
YELLOW=\033[1;33m
NC=\033[0m # No Color

help: ## Показать справку по командам
	@echo "$(GREEN)Доступные команды:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-20s$(NC) %s\n", $$1, $$2}'

build: ## Собрать бинарный файл
	@echo "$(GREEN)Сборка бинарного файла...$(NC)"
	go build -o $(BINARY_NAME) $(MAIN_PATH)

test: ## Запустить все тесты
	@echo "$(GREEN)Запуск всех тестов...$(NC)"
	go test -v ./...

test-unit: ## Запустить только юнит-тесты
	@echo "$(GREEN)Запуск юнит-тестов...$(NC)"
	go test -v -tags=!integration ./...

test-integration: ## Запустить интеграционные тесты (требует Docker)
	@echo "$(GREEN)Запуск интеграционных тестов...$(NC)"
	@echo "$(YELLOW)Убедитесь, что Docker запущен и контейнеры подняты$(NC)"
	go test -v -tags=integration ./...

test-coverage: ## Запустить тесты с покрытием
	@echo "$(GREEN)Запуск тестов с покрытием...$(NC)"
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Отчет о покрытии сохранен в coverage.html$(NC)"

run-example: ## Запустить пример использования
	@echo "$(GREEN)Запуск примера...$(NC)"
	@echo "$(YELLOW)Убедитесь, что MySQL и Kafka запущены$(NC)"
	go run $(MAIN_PATH)

docker-up: ## Поднять Docker контейнеры
	@echo "$(GREEN)Поднятие Docker контейнеров...$(NC)"
	docker-compose -f $(DOCKER_COMPOSE_FILE) up -d
	@echo "$(GREEN)Ожидание готовности сервисов...$(NC)"
	@echo "$(YELLOW)MySQL будет доступен на localhost:3306$(NC)"
	@echo "$(YELLOW)Kafka будет доступна на localhost:9092$(NC)"
	@echo "$(YELLOW)Kafka UI будет доступен на http://localhost:8080$(NC)"

docker-down: ## Остановить Docker контейнеры
	@echo "$(GREEN)Остановка Docker контейнеров...$(NC)"
	docker-compose -f $(DOCKER_COMPOSE_FILE) down

docker-logs: ## Показать логи Docker контейнеров
	@echo "$(GREEN)Логи контейнеров:$(NC)"
	docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f

docker-clean: ## Очистить Docker контейнеры и volumes
	@echo "$(GREEN)Очистка Docker контейнеров и volumes...$(NC)"
	docker-compose -f $(DOCKER_COMPOSE_FILE) down -v --remove-orphans

clean: ## Очистить скомпилированные файлы
	@echo "$(GREEN)Очистка...$(NC)"
	rm -f $(BINARY_NAME)
	rm -f coverage.out
	rm -f coverage.html

deps: ## Установить зависимости
	@echo "$(GREEN)Установка зависимостей...$(NC)"
	go mod download
	go mod tidy

lint: ## Запустить линтер
	@echo "$(GREEN)Проверка кода линтером...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "$(YELLOW)golangci-lint не установлен. Установите: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest$(NC)"; \
	fi

format: ## Форматировать код
	@echo "$(GREEN)Форматирование кода...$(NC)"
	go fmt ./...
	go vet ./...

dev-setup: ## Настройка окружения для разработки
	@echo "$(GREEN)Настройка окружения для разработки...$(NC)"
	@echo "$(YELLOW)1. Поднятие Docker контейнеров...$(NC)"
	$(MAKE) docker-up
	@echo "$(YELLOW)2. Установка зависимостей...$(NC)"
	$(MAKE) deps
	@echo "$(YELLOW)3. Запуск тестов...$(NC)"
	$(MAKE) test-unit
	@echo "$(GREEN)Окружение готово!$(NC)"
	@echo "$(YELLOW)Теперь можно запустить: make run-example$(NC)"

full-test: ## Полный цикл тестирования
	@echo "$(GREEN)Полный цикл тестирования...$(NC)"
	$(MAKE) docker-up
	@echo "$(YELLOW)Ожидание готовности сервисов...$(NC)"
	sleep 30
	$(MAKE) test
	$(MAKE) docker-down

# Команды для работы с базой данных
db-connect: ## Подключиться к MySQL
	@echo "$(GREEN)Подключение к MySQL...$(NC)"
	@echo "$(YELLOW)Используйте: mysql -h localhost -P 3306 -u outbox_user -poutbox_pass outbox_db$(NC)"
	mysql -h localhost -P 3306 -u outbox_user -poutbox_pass outbox_db

db-reset: ## Сбросить базу данных
	@echo "$(GREEN)Сброс базы данных...$(NC)"
	@echo "$(YELLOW)Убедитесь, что контейнеры запущены$(NC)"
	docker exec outbox_mysql mysql -u root -ppassword -e "DROP DATABASE IF EXISTS outbox_db; CREATE DATABASE outbox_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

# Команды для работы с Kafka
kafka-topics: ## Показать топики Kafka
	@echo "$(GREEN)Топики Kafka:$(NC)"
	docker exec outbox_kafka kafka-topics --bootstrap-server localhost:9092 --list

kafka-consumer: ## Запустить консольный consumer для топика outbox_events
	@echo "$(GREEN)Запуск Kafka consumer...$(NC)"
	docker exec -it outbox_kafka kafka-console-consumer --bootstrap-server localhost:9092 --topic outbox_events --from-beginning

# Информация
info: ## Показать информацию о проекте
	@echo "$(GREEN)Информация о проекте:$(NC)"
	@echo "  Версия Go: $(shell go version)"
	@echo "  Путь к модулю: $(shell go list -m)"
	@echo "  Зависимости: $(shell go list -m all | wc -l)"
	@echo "  Размер кода: $(shell find . -name "*.go" -not -path "./vendor/*" | xargs wc -l | tail -1)"
