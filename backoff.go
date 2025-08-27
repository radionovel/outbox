package outbox

import (
	"math"
	"math/rand"
	"time"
)

// BackoffStrategy определяет функцию для вычисления задержки между попытками
type BackoffStrategy func(attempt int) time.Duration

// ExponentialBackoffWithJitter создает стратегию экспоненциального бэкоффа с джиттером
func ExponentialBackoffWithJitter(baseDelay time.Duration, maxDelay time.Duration) BackoffStrategy {
	return func(attempt int) time.Duration {
		if attempt <= 0 {
			return baseDelay
		}

		// Экспоненциальная задержка: baseDelay * 2^(attempt-1)
		exponentialDelay := float64(baseDelay) * math.Pow(2, float64(attempt-1))
		
		// Ограничиваем максимальной задержкой
		if exponentialDelay > float64(maxDelay) {
			exponentialDelay = float64(maxDelay)
		}

		// Добавляем джиттер: ±25% от вычисленной задержки
		jitterFactor := 0.75 + rand.Float64()*0.5 // 0.75 до 1.25
		delayWithJitter := exponentialDelay * jitterFactor

		return time.Duration(delayWithJitter)
	}
}

// DefaultBackoffStrategy возвращает стандартную стратегию бэкоффа
func DefaultBackoffStrategy() BackoffStrategy {
	return ExponentialBackoffWithJitter(
		100*time.Millisecond,  // базовая задержка
		30*time.Second,        // максимальная задержка
	)
}

// LinearBackoffWithJitter создает линейную стратегию бэкоффа с джиттером
func LinearBackoffWithJitter(baseDelay time.Duration, maxDelay time.Duration) BackoffStrategy {
	return func(attempt int) time.Duration {
		if attempt <= 0 {
			return baseDelay
		}

		// Линейная задержка: baseDelay * attempt
		linearDelay := float64(baseDelay) * float64(attempt)
		
		// Ограничиваем максимальной задержкой
		if linearDelay > float64(maxDelay) {
			linearDelay = float64(maxDelay)
		}

		// Добавляем джиттер: ±20% от вычисленной задержки
		jitterFactor := 0.8 + rand.Float64()*0.4 // 0.8 до 1.2
		delayWithJitter := linearDelay * jitterFactor

		return time.Duration(delayWithJitter)
	}
}
