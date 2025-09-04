# Outbox Pattern Example

Этот пример демонстрирует использование outbox паттерна для обеспечения надежной доставки событий в распределенных системах.

## Что такое Outbox Pattern?

Outbox Pattern - это паттерн, который решает проблему обеспечения консистентности между базой данных и внешними системами (например, Kafka). Основная идея заключается в том, что события сохраняются в той же транзакции, что и бизнес-данные, а затем асинхронно публикуются во внешние системы.

## Архитектура примера

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
                       │ - Poll events   │
                       │ - Publish to    │
                       │   Kafka         │
                       │ - Handle retries│
                       │ - Dead letters  │
                       └─────────────────┘
```

## Компоненты

### 1. Event (Событие)
- `EventID` - уникальный идентификатор события
- `EventType` - тип события (например, "UserCreated")
- `AggregateType` - тип агрегата (например, "User")
- `AggregateID` - идентификатор агрегата
- `Topic` - топик Kafka для публикации
- `Payload` - данные события в формате JSON
- `TraceID`/`SpanID` - для трейсинга

### 2. Dispatcher
- Обрабатывает события из таблицы `outbox_events`
- Публикует события в Kafka
- Обрабатывает повторные попытки
- Управляет dead letter queue
- Очищает старые события

### 3. Services
- **EventProcessor** - основная обработка событий
- **DeadLetterService** - обработка неудачных событий
- **StuckEventService** - восстановление "застрявших" событий
- **CleanupService** - очистка старых событий

## Запуск примера

### Предварительные требования

1. Docker и Docker Compose
2. Go 1.23+

### Шаги запуска

1. **Запустите инфраструктуру:**
   ```bash
   docker-compose up -d
   ```

2. **Дождитесь готовности сервисов:**
   ```bash
   docker-compose ps
   ```

3. **Запустите пример:**
   ```bash
   cd example
   go run .
   ```

### Что происходит при запуске

1. Создаются таблицы `outbox_events` и `outbox_deadletters`
2. Запускается dispatcher с 4 воркерами:
   - Event Processor (каждые 2 сек)
   - Dead Letter Processor (каждые 5 мин)
   - Stuck Events Processor (каждые 2 мин)
   - Cleanup Processor (каждый час)
3. Создаются тестовые события в транзакции
4. Dispatcher автоматически обрабатывает и публикует события
5. Каждые 5 секунд выводятся метрики

## Конфигурация

### База данных
- Host: localhost:3306
- Database: outbox_db
- User: outbox_user
- Password: outbox_pass

### Kafka
- Brokers: localhost:9092
- Topic: user-events (по умолчанию)
- UI: http://localhost:8080

### Outbox настройки
- Batch Size: 10 событий
- Poll Interval: 2 секунды
- Max Attempts: 3 попытки
- Dead Letter Retention: 7 дней
- Sent Events Retention: 1 день

## Мониторинг

### Логи
Приложение выводит подробные логи о:
- Создании событий
- Обработке событий
- Публикации в Kafka
- Ошибках и повторных попытках
- Метриках работы

### Kafka UI
Откройте http://localhost:8080 для просмотра:
- Топиков и сообщений
- Consumer groups
- Метрик Kafka

### Метрики
Каждые 5 секунд выводятся метрики:
- Статус dispatcher'а
- Количество воркеров
- Конфигурация

## Примеры событий

В примере создаются следующие события:

1. **UserCreated** - создание пользователя
2. **UserUpdated** - обновление пользователя  
3. **OrderCreated** - создание заказа

Каждое событие содержит:
- Уникальный ID
- Тип события
- Данные в JSON формате
- Временные метки
- Trace ID для трейсинга

## Обработка ошибок

### Retry Strategy
- Экспоненциальная задержка между попытками
- Максимум 3 попытки
- Базовая задержка: 1 минута
- Максимальная задержка: 30 минут

### Dead Letter Queue
- События, которые не удалось обработать после всех попыток
- Сохраняются в таблице `outbox_deadletters`
- Автоматическая очистка через 7 дней

### Stuck Events
- События в статусе "processing" более 10 минут
- Автоматически переводятся в статус "retry"
- Проверка каждые 2 минуты

## Остановка

Нажмите `Ctrl+C` для корректной остановки:
- Останавливается dispatcher
- Завершаются все воркеры
- Закрываются соединения

## Расширение примера

### Добавление новых событий
```go
event, err := outbox.NewOutboxEvent(
    "evt-004",
    "ProductCreated",
    "Product", 
    "product-789",
    "product-events",
    map[string]interface{}{
        "product_id": "product-789",
        "name": "Example Product",
        "price": 29.99,
    },
)
```

### Кастомная конфигурация
```go
dispatcher := outbox.NewDispatcher(db,
    outbox.WithBatchSize(20),
    outbox.WithPollInterval(1*time.Second),
    outbox.WithMaxAttempts(5),
    outbox.WithLogger(customLogger),
)
```

### Кастомный Publisher
```go
type CustomPublisher struct{}

func (p *CustomPublisher) Publish(ctx context.Context, event outbox.EventRecord) error {
    // Ваша логика публикации
    return nil
}

dispatcher := outbox.NewDispatcher(db,
    outbox.WithPublisher(&CustomPublisher{}),
)
```
