package resilience

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRetrySucceedsAfterTransientFailures(t *testing.T) {
	p := NewRetryPolicy(5, time.Millisecond, 10*time.Millisecond)
	calls := 0
	got, err := Retry(context.Background(), p, func(ctx context.Context) (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("transient")
		}
		return 42, nil
	})
	if err != nil || got != 42 {
		t.Fatalf("Retry = %d, %v", got, err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryStopsOnNonRetryable(t *testing.T) {
	sentinel := errors.New("fatal")
	p := NewRetryPolicy(5, time.Millisecond, 10*time.Millisecond)
	p.Retryable = func(err error) bool { return !errors.Is(err, sentinel) }
	calls := 0
	_, err := Retry(context.Background(), p, func(ctx context.Context) (int, error) {
		calls++
		return 0, sentinel
	})
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("expected 1 call and sentinel, got %d calls %v", calls, err)
	}
}

func TestRetryExhausts(t *testing.T) {
	p := NewRetryPolicy(3, time.Millisecond, 5*time.Millisecond)
	calls := 0
	_, err := Retry(context.Background(), p, func(ctx context.Context) (int, error) {
		calls++
		return 0, errors.New("always")
	})
	if err == nil || calls != 3 {
		t.Fatalf("expected exhaustion after 3, got %d calls %v", calls, err)
	}
}

func TestRetryHonoursContext(t *testing.T) {
	p := NewRetryPolicy(10, 50*time.Millisecond, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Retry(ctx, p, func(ctx context.Context) (int, error) {
		return 0, errors.New("x")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestCircuitBreakerTripsAndRecovers(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	transitions := []string{}
	cb := NewCircuitBreaker(BreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Cooldown:         time.Second,
		Now:              clock,
		OnStateChange:    func(from, to State) { transitions = append(transitions, to.String()) },
	})

	failOp := func(ctx context.Context) (int, error) { return 0, errors.New("boom") }
	okOp := func(ctx context.Context) (int, error) { return 1, nil }

	// two failures trip the breaker
	_, _ = Execute(context.Background(), cb, failOp)
	_, _ = Execute(context.Background(), cb, failOp)
	if cb.State() != StateOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}
	// fast-fail while open
	if _, err := Execute(context.Background(), cb, okOp); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
	// after cooldown -> half-open -> success closes it
	now = now.Add(2 * time.Second)
	if _, err := Execute(context.Background(), cb, okOp); err != nil {
		t.Fatalf("probe should succeed, got %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected closed, got %s", cb.State())
	}
}

func TestCircuitBreakerHalfOpenReopensOnFailure(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	cb := NewCircuitBreaker(BreakerConfig{FailureThreshold: 1, Cooldown: time.Second, Now: clock})
	failOp := func(ctx context.Context) (int, error) { return 0, errors.New("boom") }
	_, _ = Execute(context.Background(), cb, failOp)
	if cb.State() != StateOpen {
		t.Fatalf("expected open")
	}
	now = now.Add(2 * time.Second)
	// probe fails -> back to open
	_, _ = Execute(context.Background(), cb, failOp)
	if cb.State() != StateOpen {
		t.Fatalf("expected re-open, got %s", cb.State())
	}
}

func TestRateLimiter(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	rl := newRateLimiterWithClock(10, 2, clock)
	if !rl.Allow() || !rl.Allow() {
		t.Fatal("first two calls should be allowed (burst=2)")
	}
	if rl.Allow() {
		t.Fatal("third immediate call should be denied")
	}
	// advance 0.1s -> 1 token refills at 10/s
	now = now.Add(100 * time.Millisecond)
	if !rl.Allow() {
		t.Fatal("token should have refilled")
	}
}

func TestGuardRateLimited(t *testing.T) {
	now := time.Now()
	rl := newRateLimiterWithClock(1, 1, func() time.Time { return now })
	op := func(ctx context.Context) (int, error) { return 7, nil }
	if v, err := Guard(context.Background(), rl, op); err != nil || v != 7 {
		t.Fatalf("first guard = %d, %v", v, err)
	}
	if _, err := Guard(context.Background(), rl, op); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestBulkhead(t *testing.T) {
	b := NewBulkhead(1)
	release := make(chan struct{})
	entered := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = RunBulkhead(context.Background(), b, func(ctx context.Context) (int, error) {
			close(entered)
			<-release
			return 0, nil
		})
	}()
	<-entered
	// second concurrent call should be rejected fast
	if _, err := RunBulkhead(context.Background(), b, func(ctx context.Context) (int, error) { return 1, nil }); !errors.Is(err, ErrBulkheadFull) {
		t.Fatalf("expected ErrBulkheadFull, got %v", err)
	}
	close(release)
	wg.Wait()
	// now a slot is free
	if v, err := RunBulkhead(context.Background(), b, func(ctx context.Context) (int, error) { return 9, nil }); err != nil || v != 9 {
		t.Fatalf("post-release = %d, %v", v, err)
	}
}

func TestWithTimeout(t *testing.T) {
	// fast op returns before timeout
	v, err := WithTimeout(context.Background(), 100*time.Millisecond, func(ctx context.Context) (int, error) {
		return 5, nil
	})
	if err != nil || v != 5 {
		t.Fatalf("fast op = %d, %v", v, err)
	}
	// slow op hits the deadline
	_, err = WithTimeout(context.Background(), 10*time.Millisecond, func(ctx context.Context) (int, error) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Second):
			return 1, nil
		}
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
