package resilience

import (
	"context"
	"sync"
	"time"
)

// State is the circuit-breaker state.
type State int

// The three breaker states.
const (
	StateClosed   State = iota // calls flow; failures are counted
	StateOpen                  // calls fail fast until the cool-down elapses
	StateHalfOpen              // a probe call is allowed to test recovery
)

// String renders the state for logging/metrics.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker trips open after consecutive failures to shed load from a
// failing dependency, then probes for recovery after a cool-down. It is safe
// for concurrent use.
type CircuitBreaker struct {
	mu               sync.Mutex
	state            State
	failures         int
	failureThreshold int
	successThreshold int
	halfOpenSuccess  int
	cooldown         time.Duration
	openedAt         time.Time
	now              func() time.Time
	onStateChange    func(from, to State)
}

// BreakerConfig configures a CircuitBreaker.
type BreakerConfig struct {
	FailureThreshold int           // consecutive failures that trip the breaker
	SuccessThreshold int           // half-open successes that close it again
	Cooldown         time.Duration // how long to stay open before probing
	Now              func() time.Time
	OnStateChange    func(from, to State)
}

// NewCircuitBreaker builds a breaker, applying defaults for zero fields.
func NewCircuitBreaker(cfg BreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold < 1 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold < 1 {
		cfg.SuccessThreshold = 1
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 5 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: cfg.FailureThreshold,
		successThreshold: cfg.SuccessThreshold,
		cooldown:         cfg.Cooldown,
		now:              cfg.Now,
		onStateChange:    cfg.OnStateChange,
	}
}

// State returns the current breaker state (transitioning to half-open if the
// cool-down has elapsed).
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.refresh()
	return cb.state
}

// refresh promotes an open breaker to half-open once the cool-down passes.
// Caller must hold the lock.
func (cb *CircuitBreaker) refresh() {
	if cb.state == StateOpen && cb.now().Sub(cb.openedAt) >= cb.cooldown {
		cb.transition(StateHalfOpen)
		cb.halfOpenSuccess = 0
	}
}

// transition changes state and fires the observer. Caller must hold the lock.
func (cb *CircuitBreaker) transition(to State) {
	if cb.state == to {
		return
	}
	from := cb.state
	cb.state = to
	if cb.onStateChange != nil {
		cb.onStateChange(from, to)
	}
}

// Execute runs op subject to the breaker, returning ErrCircuitOpen when the
// breaker is open. Generic over the result type so no interface boxing occurs.
func Execute[T any](ctx context.Context, cb *CircuitBreaker, op Operation[T]) (T, error) {
	var zero T
	if !cb.allow() {
		return zero, ErrCircuitOpen
	}
	result, err := op(ctx)
	cb.record(err)
	if err != nil {
		return zero, err
	}
	return result, nil
}

// allow reports whether a call may proceed and reserves a half-open probe.
func (cb *CircuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.refresh()
	return cb.state != StateOpen
}

// record folds a call outcome back into the breaker state.
func (cb *CircuitBreaker) record(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err != nil {
		cb.failures++
		switch cb.state {
		case StateHalfOpen:
			cb.trip()
		case StateClosed:
			if cb.failures >= cb.failureThreshold {
				cb.trip()
			}
		}
		return
	}
	switch cb.state {
	case StateHalfOpen:
		cb.halfOpenSuccess++
		if cb.halfOpenSuccess >= cb.successThreshold {
			cb.transition(StateClosed)
			cb.failures = 0
		}
	case StateClosed:
		cb.failures = 0
	}
}

// trip opens the breaker and stamps the open time. Caller must hold the lock.
func (cb *CircuitBreaker) trip() {
	cb.transition(StateOpen)
	cb.openedAt = cb.now()
	cb.failures = 0
}
