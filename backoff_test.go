package outbox

import (
	"testing"
	"time"
)

func TestExponentialBackoffWithJitter(t *testing.T) {
	baseDelay := 100 * time.Millisecond
	maxDelay := 1 * time.Second
	strategy := ExponentialBackoffWithJitter(baseDelay, maxDelay)

	tests := []struct {
		attempt    int
		expectMin  time.Duration
		expectMax  time.Duration
		description string
	}{
		{
			attempt:     0,
			expectMin:   baseDelay,
			expectMax:   baseDelay,
			description: "zero attempt should return base delay",
		},
		{
			attempt:     1,
			expectMin:   baseDelay * 75 / 100,        // 0.75 * baseDelay
			expectMax:   baseDelay * 125 / 100,       // 1.25 * baseDelay
			description: "first attempt should be around base delay with jitter",
		},
		{
			attempt:     2,
			expectMin:   baseDelay * 2 * 75 / 100,   // 0.75 * 2 * baseDelay
			expectMax:   baseDelay * 2 * 125 / 100,  // 1.25 * 2 * baseDelay
			description: "second attempt should be around 2 * base delay with jitter",
		},
		{
			attempt:     5,
			expectMin:   baseDelay * 16 * 75 / 100,  // 0.75 * 2^4 * baseDelay
			expectMax:   maxDelay,                    // Should be capped at maxDelay
			description: "fifth attempt should be capped at max delay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			delay := strategy(tt.attempt)
			
			if delay < tt.expectMin {
				t.Errorf("attempt %d: delay %v is less than expected minimum %v", 
					tt.attempt, delay, tt.expectMin)
			}
			
			if delay > tt.expectMax {
				t.Errorf("attempt %d: delay %v is greater than expected maximum %v", 
					tt.attempt, delay, tt.expectMax)
			}
		})
	}
}

func TestLinearBackoffWithJitter(t *testing.T) {
	baseDelay := 100 * time.Millisecond
	maxDelay := 1 * time.Second
	strategy := LinearBackoffWithJitter(baseDelay, maxDelay)

	tests := []struct {
		attempt    int
		expectMin  time.Duration
		expectMax  time.Duration
		description string
	}{
		{
			attempt:     0,
			expectMin:   baseDelay,
			expectMax:   baseDelay,
			description: "zero attempt should return base delay",
		},
		{
			attempt:     1,
			expectMin:   baseDelay * 80 / 100,        // 0.8 * baseDelay
			expectMax:   baseDelay * 120 / 100,       // 1.2 * baseDelay
			description: "first attempt should be around base delay with jitter",
		},
		{
			attempt:     2,
			expectMin:   baseDelay * 2 * 80 / 100,   // 0.8 * 2 * baseDelay
			expectMax:   baseDelay * 2 * 120 / 100,  // 1.2 * 2 * baseDelay
			description: "second attempt should be around 2 * base delay with jitter",
		},
		{
			attempt:     10,
			expectMin:   baseDelay * 10 * 80 / 100,  // 0.8 * 10 * baseDelay
			expectMax:   maxDelay,                    // Should be capped at maxDelay
			description: "tenth attempt should be capped at max delay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			delay := strategy(tt.attempt)
			
			if delay < tt.expectMin {
				t.Errorf("attempt %d: delay %v is less than expected minimum %v", 
					tt.attempt, delay, tt.expectMin)
			}
			
			if delay > tt.expectMax {
				t.Errorf("attempt %d: delay %v is greater than expected maximum %v", 
					tt.attempt, delay, tt.expectMax)
			}
		})
	}
}

func TestDefaultBackoffStrategy(t *testing.T) {
	strategy := DefaultBackoffStrategy()
	
	// Проверяем, что стратегия не nil
	if strategy == nil {
		t.Fatal("DefaultBackoffStrategy returned nil")
	}

	// Проверяем базовое поведение
	delay := strategy(1)
	if delay <= 0 {
		t.Errorf("Default strategy returned non-positive delay: %v", delay)
	}

	// Проверяем, что задержка увеличивается с попытками
	delay1 := strategy(1)
	delay2 := strategy(2)
	delay3 := strategy(3)

	if delay1 >= delay2 {
		t.Errorf("Delay should increase with attempts: %v >= %v", delay1, delay2)
	}
	if delay2 >= delay3 {
		t.Errorf("Delay should increase with attempts: %v >= %v", delay2, delay3)
	}
}

func TestBackoffStrategyConsistency(t *testing.T) {
	baseDelay := 50 * time.Millisecond
	maxDelay := 500 * time.Millisecond
	
	// Тестируем консистентность для одной и той же попытки
	strategy := ExponentialBackoffWithJitter(baseDelay, maxDelay)
	
	attempt := 3
	delays := make([]time.Duration, 10)
	
	for i := 0; i < 10; i++ {
		delays[i] = strategy(attempt)
	}
	
	// Проверяем, что все задержки находятся в разумных пределах
	expectedMin := baseDelay * 4 * 75 / 100  // 0.75 * 2^2 * baseDelay
	expectedMax := baseDelay * 4 * 125 / 100 // 1.25 * 2^2 * baseDelay
	
	for i, delay := range delays {
		if delay < expectedMin || delay > expectedMax {
			t.Errorf("Delay %d for attempt %d (%v) is outside expected range [%v, %v]", 
				i, attempt, delay, expectedMin, expectedMax)
		}
	}
}
