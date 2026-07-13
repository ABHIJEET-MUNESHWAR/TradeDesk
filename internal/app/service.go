// Package app is the application core: it orchestrates domain behaviour behind a
// CQRS command/query API and guards every I/O boundary with resilience
// primitives (rate limit, circuit breaker, timeout, retry). It depends only on
// ports, never on concrete adapters.
package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/domain"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/observability"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/ports"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/resilience"
)

// Config tunes the service's resilience behaviour.
type Config struct {
	RiskTimeout     time.Duration
	RouteTimeout    time.Duration
	RetryAttempts   int
	RetryBase       time.Duration
	RetryMax        time.Duration
	RateLimitRPS    float64
	RateLimitBurst  int
	BreakerFailures int
	BreakerCooldown time.Duration
}

// Deps are the injected ports the service depends on.
type Deps struct {
	Repo      ports.OrderRepository
	Publisher ports.EventPublisher
	Risk      ports.RiskGate
	Venue     ports.Venue
	Positions ports.PositionReadModel
	Clock     ports.Clock
	IDs       ports.IDGenerator
	Logger    *slog.Logger
	Metrics   *observability.Metrics
}

// Service is the application service implementing the command and query buses.
type Service struct {
	deps    Deps
	cfg     Config
	breaker *resilience.CircuitBreaker
	retry   *resilience.RetryPolicy
	limiter *resilience.RateLimiter
}

// NewService wires the service, constructing its resilience primitives from cfg.
func NewService(d Deps, cfg Config) *Service {
	breaker := resilience.NewCircuitBreaker(resilience.BreakerConfig{
		FailureThreshold: cfg.BreakerFailures,
		Cooldown:         cfg.BreakerCooldown,
		Now:              d.Clock.Now,
	})
	retry := resilience.NewRetryPolicy(cfg.RetryAttempts, cfg.RetryBase, cfg.RetryMax)
	limiter := resilience.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)
	return &Service{deps: d, cfg: cfg, breaker: breaker, retry: retry, limiter: limiter}
}

// PlaceOrder runs the full write pipeline: rate-limit -> idempotency -> build ->
// pre-trade risk (breaker + timeout) -> persist -> publish -> route (retry).
func (s *Service) PlaceOrder(ctx context.Context, cmd PlaceOrderCommand) (PlaceOrderResult, error) {
	start := s.deps.Clock.Now()
	defer func() {
		s.deps.Metrics.PlaceLatency.Observe(s.deps.Clock.Now().Sub(start).Seconds())
	}()

	if !s.limiter.Allow() {
		return PlaceOrderResult{}, resilience.ErrRateLimited
	}
	if cmd.IdempotencyKey != "" {
		if existing, ok := s.deps.Repo.FindByIdempotencyKey(ctx, cmd.IdempotencyKey); ok {
			return PlaceOrderResult{OrderID: existing, Idempotent: true}, nil
		}
	}
	order, err := s.build(cmd)
	if err != nil {
		return PlaceOrderResult{}, err
	}

	decision, riskErr := resilience.Execute(ctx, s.breaker, func(c context.Context) (ports.RiskDecision, error) {
		return resilience.WithTimeout(c, s.cfg.RiskTimeout, func(c2 context.Context) (ports.RiskDecision, error) {
			return s.deps.Risk.Check(c2, order)
		})
	})
	now := s.deps.Clock.Now()
	switch {
	case riskErr != nil:
		// The risk system is unavailable: fail closed and reject the order.
		_ = order.Reject("risk check unavailable", now)
		s.deps.Metrics.RiskChecks.WithLabelValues("error").Inc()
		s.deps.Logger.Warn("risk check failed; rejecting order", "err", riskErr)
	case decision.Approved:
		_ = order.Accept(now)
		s.deps.Metrics.RiskChecks.WithLabelValues("approved").Inc()
	default:
		_ = order.Reject(decision.Reason, now)
		s.deps.Metrics.RiskChecks.WithLabelValues("rejected").Inc()
	}

	if err := s.deps.Repo.Save(ctx, order, cmd.IdempotencyKey); err != nil {
		return PlaceOrderResult{}, err
	}
	s.deps.Metrics.OrdersPlaced.WithLabelValues(string(order.Status())).Inc()
	s.deps.Publisher.Publish(ctx, order.PullEvents()...)

	if order.Status() == domain.StatusAccepted {
		if err := s.route(ctx, order); err != nil {
			s.deps.Metrics.RouteFailures.Inc()
			s.deps.Logger.Warn("venue routing failed", "order_id", order.ID(), "err", err)
		}
	}
	return PlaceOrderResult{OrderID: order.ID(), Status: string(order.Status())}, nil
}

// build parses and validates a command into a new domain order.
func (s *Service) build(cmd PlaceOrderCommand) (*domain.Order, error) {
	symbol, err := domain.ParseSymbol(cmd.Symbol)
	if err != nil {
		return nil, err
	}
	side, err := domain.ParseSide(cmd.Side)
	if err != nil {
		return nil, err
	}
	ot, err := domain.ParseOrderType(cmd.OrderType)
	if err != nil {
		return nil, err
	}
	tif, err := domain.ParseTimeInForce(cmd.TIF)
	if err != nil {
		return nil, err
	}
	qty, err := domain.ParseQuantity(cmd.Quantity)
	if err != nil {
		return nil, err
	}
	var limit domain.Money
	if ot == domain.OrderTypeLimit {
		if limit, err = domain.ParseMoney(cmd.LimitPrice); err != nil {
			return nil, err
		}
	}
	return domain.NewOrder(s.deps.IDs.NewID(), cmd.AccountID, symbol, side, ot, tif, limit, qty, s.deps.Clock.Now())
}

// route sends an accepted order to the venue with retry + per-attempt timeout.
func (s *Service) route(ctx context.Context, o *domain.Order) error {
	_, err := resilience.Retry(ctx, s.retry, func(c context.Context) (struct{}, error) {
		return resilience.WithTimeout(c, s.cfg.RouteTimeout, func(c2 context.Context) (struct{}, error) {
			return struct{}{}, s.deps.Venue.Route(c2, o)
		})
	})
	return err
}

// CancelOrder cancels a working order under optimistic concurrency.
func (s *Service) CancelOrder(ctx context.Context, cmd CancelOrderCommand) error {
	order, err := s.deps.Repo.Get(ctx, cmd.OrderID)
	if err != nil {
		return err
	}
	expected := order.Version()
	if err := order.Cancel(s.deps.Clock.Now()); err != nil {
		return err
	}
	if err := s.deps.Repo.Update(ctx, order, expected); err != nil {
		return err
	}
	s.deps.Metrics.OrdersCanceled.Inc()
	s.deps.Publisher.Publish(ctx, order.PullEvents()...)
	return nil
}

// ApplyFill records an execution against an order under optimistic concurrency.
func (s *Service) ApplyFill(ctx context.Context, cmd ApplyFillCommand) error {
	qty, err := domain.ParseQuantity(cmd.Quantity)
	if err != nil {
		return err
	}
	px, err := domain.ParseMoney(cmd.Price)
	if err != nil {
		return err
	}
	order, err := s.deps.Repo.Get(ctx, cmd.OrderID)
	if err != nil {
		return err
	}
	expected := order.Version()
	if err := order.Fill(qty, px, s.deps.Clock.Now()); err != nil {
		return err
	}
	if err := s.deps.Repo.Update(ctx, order, expected); err != nil {
		return err
	}
	if order.Status() == domain.StatusFilled {
		s.deps.Metrics.OrdersFilled.Inc()
	}
	s.deps.Publisher.Publish(ctx, order.PullEvents()...)
	return nil
}

// GetOrder returns a single order view by id.
func (s *Service) GetOrder(ctx context.Context, id string) (OrderView, error) {
	o, err := s.deps.Repo.Get(ctx, id)
	if err != nil {
		return OrderView{}, err
	}
	return viewFromOrder(o), nil
}

// ListOrders returns recent order views, newest first.
func (s *Service) ListOrders(ctx context.Context, limit int) ([]OrderView, error) {
	orders, err := s.deps.Repo.List(ctx, limit)
	if err != nil {
		return nil, err
	}
	views := make([]OrderView, len(orders))
	for i, o := range orders {
		views[i] = viewFromOrder(o)
	}
	return views, nil
}

// Positions serves the account's holdings from the read model.
func (s *Service) Positions(ctx context.Context, accountID string) ([]ports.PositionView, error) {
	return s.deps.Positions.Positions(ctx, accountID)
}

// BreakerState exposes the current risk-breaker state for health/metrics.
func (s *Service) BreakerState() string { return s.breaker.State().String() }
