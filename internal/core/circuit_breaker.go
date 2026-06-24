package core

import (
	"fmt"
	"sync"
	"time"
)

const (
	defaultCircuitBreakerThreshold = 5
	defaultCircuitBreakerCooldown  = 10 * time.Second
)

type CircuitBreaker struct {
	mu        sync.Mutex
	failures  int
	openedAt  time.Time
	threshold int
	cooldown  time.Duration
}

func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		threshold: defaultCircuitBreakerThreshold,
		cooldown:  defaultCircuitBreakerCooldown,
	}
}

func (b *CircuitBreaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.failures < b.threshold {
		return nil
	}
	if time.Since(b.openedAt) >= b.cooldown {
		return nil
	}
	return fmt.Errorf("circuit breaker open")
}

func (b *CircuitBreaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures = 0
	b.openedAt = time.Time{}
}

func (b *CircuitBreaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures++
	if b.failures == b.threshold {
		b.openedAt = time.Now()
	}
}
