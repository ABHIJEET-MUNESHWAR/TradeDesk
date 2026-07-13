package resilience

import (
	"context"
	"sync"
	"time"
)

// RateLimiter is a thread-safe token-bucket limiter. Tokens refill at a steady
// rate up to a burst capacity; each admitted call consumes one token.
type RateLimiter struct {
	mu         sync.Mutex
	capacity   float64
	tokens     float64
	refillRate float64 // tokens per second
	last       time.Time
	now        func() time.Time
}

// NewRateLimiter builds a limiter allowing refillPerSec sustained calls with a
// burst up to capacity.
func NewRateLimiter(refillPerSec float64, capacity int) *RateLimiter {
	return newRateLimiterWithClock(refillPerSec, capacity, time.Now)
}

func newRateLimiterWithClock(refillPerSec float64, capacity int, now func() time.Time) *RateLimiter {
	if capacity < 1 {
		capacity = 1
	}
	if refillPerSec <= 0 {
		refillPerSec = 1
	}
	return &RateLimiter{
		capacity:   float64(capacity),
		tokens:     float64(capacity),
		refillRate: refillPerSec,
		last:       now(),
		now:        now,
	}
}

// Allow reports whether a call may proceed, consuming a token if so.
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refill()
	if r.tokens >= 1 {
		r.tokens--
		return true
	}
	return false
}

// refill adds tokens accrued since the last call. Caller must hold the lock.
func (r *RateLimiter) refill() {
	now := r.now()
	elapsed := now.Sub(r.last).Seconds()
	if elapsed <= 0 {
		return
	}
	r.tokens += elapsed * r.refillRate
	if r.tokens > r.capacity {
		r.tokens = r.capacity
	}
	r.last = now
}

// Guard wraps op, returning ErrRateLimited when the bucket is empty.
func Guard[T any](ctx context.Context, r *RateLimiter, op Operation[T]) (T, error) {
	var zero T
	if !r.Allow() {
		return zero, ErrRateLimited
	}
	return op(ctx)
}

// Bulkhead bounds the number of concurrent in-flight operations, isolating a
// slow dependency so it cannot exhaust the whole worker pool.
type Bulkhead struct {
	slots chan struct{}
}

// NewBulkhead builds a bulkhead admitting at most maxConcurrent calls.
func NewBulkhead(maxConcurrent int) *Bulkhead {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Bulkhead{slots: make(chan struct{}, maxConcurrent)}
}

// RunBulkhead executes op if a slot is free, else fails fast with ErrBulkheadFull.
// It honours context cancellation while waiting is disabled (fail-fast).
func RunBulkhead[T any](ctx context.Context, b *Bulkhead, op Operation[T]) (T, error) {
	var zero T
	select {
	case b.slots <- struct{}{}:
		defer func() { <-b.slots }()
		return op(ctx)
	case <-ctx.Done():
		return zero, ctx.Err()
	default:
		return zero, ErrBulkheadFull
	}
}

// WithTimeout runs op under a derived context that cancels after d.
func WithTimeout[T any](ctx context.Context, d time.Duration, op Operation[T]) (T, error) {
	cctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	type outcome struct {
		val T
		err error
	}
	ch := make(chan outcome, 1)
	go func() {
		v, err := op(cctx)
		ch <- outcome{v, err}
	}()
	select {
	case <-cctx.Done():
		var zero T
		return zero, cctx.Err()
	case o := <-ch:
		return o.val, o.err
	}
}
