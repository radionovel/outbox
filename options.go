package outbox

import (
	"time"

	"go.uber.org/zap"
)

type DispatcherOption func(*dispatcherOptions)

type dispatcherOptions struct {
	batchSize               int
	pollInterval            time.Duration
	maxAttempts             int
	deadLetterInterval      time.Duration
	stuckEventTimeout       time.Duration
	stuckEventCheckInterval time.Duration
	deadLetterRetention     time.Duration
	sentEventsRetention     time.Duration
	cleanupInterval         time.Duration
	backoffStrategy         BackoffStrategy
	publisher               Publisher
	metrics                 MetricsCollector
	logger                  *zap.Logger
}

func WithBatchSize(size int) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.batchSize = size
	}
}

func WithPollInterval(interval time.Duration) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.pollInterval = interval
	}
}

func WithMaxAttempts(attempts int) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.maxAttempts = attempts
	}
}

func WithDeadLetterInterval(interval time.Duration) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.deadLetterInterval = interval
	}
}

func WithStuckEventTimeout(timeout time.Duration) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.stuckEventTimeout = timeout
	}
}

func WithStuckEventCheckInterval(interval time.Duration) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.stuckEventCheckInterval = interval
	}
}

func WithDeadLetterRetention(retention time.Duration) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.deadLetterRetention = retention
	}
}

func WithSentEventsRetention(retention time.Duration) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.sentEventsRetention = retention
	}
}

func WithCleanupInterval(interval time.Duration) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.cleanupInterval = interval
	}
}

func WithBackoffStrategy(strategy BackoffStrategy) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.backoffStrategy = strategy
	}
}

func WithPublisher(publisher Publisher) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.publisher = publisher
	}
}

func WithMetrics(metrics MetricsCollector) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.metrics = metrics
	}
}

func WithLogger(logger *zap.Logger) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.logger = logger
	}
}

func WithKafkaConfig(config KafkaConfig) DispatcherOption {
	return func(opts *dispatcherOptions) {
		opts.publisher = NewKafkaPublisherWithConfig(opts.logger, config)
	}
}
