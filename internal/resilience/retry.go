// Package resilience provides composable, dependency-free fault-tolerance
// primitives — retry with backoff, a circuit breaker, a token-bucket rate
// limiter, and a bulkhead — used to guard every I/O boundary. Each primitive is
// generic over the result type so it composes without boxing.
package resilience

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when the breaker rejects a call fast.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// ErrRateLimited is returned when the rate limiter rejects a call.
var ErrRateLimited = errors.New("rate limit exceeded")

// ErrBulkheadFull is returned when the bulkhead has no free slot.
var ErrBulkheadFull = errors.New("bulkhead capacity exceeded")

// Operation is a cancellable unit of work returning a value of type T.
type Operation[T any] func(ctx context.Context) (T, error)

// RetryPolicy configures exponential backoff with full jitter.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	// Retryable reports whether an error is worth retrying. If nil, all
	// non-nil errors are retried.
	Retryable func(error) bool
	// rng is injectable for deterministic tests; defaults to a shared source.
	rng *rand.Rand
	mu  sync.Mutex
}

// NewRetryPolicy builds a policy with sensible defaults filled in.
func NewRetryPolicy(maxAttempts int, base, max time.Duration) *RetryPolicy {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	return &RetryPolicy{
		MaxAttempts: maxAttempts,
		BaseDelay:   base,
		MaxDelay:    max,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Retry executes op, retrying transient failures with jittered exponential
// backoff until success, a non-retryable error, exhaustion, or ctx cancels.
func Retry[T any](ctx context.Context, p *RetryPolicy, op Operation[T]) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt < p.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		result, err := op(ctx)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if p.Retryable != nil && !p.Retryable(err) {
			return zero, err
		}
		if attempt == p.MaxAttempts-1 {
			break
		}
		delay := p.backoff(attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}
	return zero, lastErr
}

// backoff computes the jittered delay for a given zero-based attempt.
func (p *RetryPolicy) backoff(attempt int) time.Duration {
	exp := float64(p.BaseDelay) * math.Pow(2, float64(attempt))
	if exp > float64(p.MaxDelay) {
		exp = float64(p.MaxDelay)
	}
	p.mu.Lock()
	jitter := p.rng.Float64()
	p.mu.Unlock()
	return time.Duration(exp * jitter)
}
