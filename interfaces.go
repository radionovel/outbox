package outbox

import (
	"context"
	"time"
)

type Publisher interface {
	Publish(ctx context.Context, event EventRecord) error
}

type EventProcessor interface {
	ProcessEvents(ctx context.Context) error
}

type DeadLetterService interface {
	MoveToDeadLetters(ctx context.Context) error
}

type StuckEventService interface {
	RecoverStuckEvents(ctx context.Context) error
}

type CleanupService interface {
	Cleanup(ctx context.Context) error
}

type MetricsCollector interface {
	IncrementCounter(name string, tags map[string]string)
	RecordDuration(name string, duration time.Duration, tags map[string]string)
	RecordGauge(name string, value float64, tags map[string]string)
}

type Worker interface {
	Start(ctx context.Context)
	Stop()
	Name() string
}
