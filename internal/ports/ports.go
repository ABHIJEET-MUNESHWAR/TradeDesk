// Package ports declares the interfaces (hexagonal ports) that the application
// core depends on. Adapters implement them, so the domain never imports a web,
// database, or messaging framework and stays unit-testable.
package ports

import (
	"context"
	"errors"
	"time"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/domain"
)

// Repository / concurrency errors surfaced by adapters.
var (
	ErrNotFound        = errors.New("ports: order not found")
	ErrVersionConflict = errors.New("ports: optimistic concurrency conflict")
	ErrDuplicate       = errors.New("ports: duplicate id")
)

// Clock abstracts wall-clock time so the core is deterministic in tests.
type Clock interface {
	Now() time.Time
}

// IDGenerator issues unique identifiers for new orders.
type IDGenerator interface {
	NewID() string
}

// OrderRepository persists orders with optimistic concurrency and idempotent
// creation. Implementations may shard/partition internally.
type OrderRepository interface {
	// Save stores a brand-new order, registering idempotencyKey (if non-empty)
	// so a retried create is not double-applied.
	Save(ctx context.Context, o *domain.Order, idempotencyKey string) error
	// Update stores a mutated order iff the persisted version equals
	// expectedVersion, else returns ErrVersionConflict.
	Update(ctx context.Context, o *domain.Order, expectedVersion int) error
	// Get returns the order by id or ErrNotFound.
	Get(ctx context.Context, id string) (*domain.Order, error)
	// FindByIdempotencyKey returns a previously-saved order id for key.
	FindByIdempotencyKey(ctx context.Context, key string) (string, bool)
	// List returns up to limit orders, newest first.
	List(ctx context.Context, limit int) ([]*domain.Order, error)
}

// EventPublisher publishes domain events to the read side (CQRS write→read).
type EventPublisher interface {
	Publish(ctx context.Context, events ...domain.Event)
}

// RiskDecision is the verdict of a pre-trade risk check.
type RiskDecision struct {
	Approved bool
	Reason   string
	// Score is an advisory 0-100 risk score (higher = riskier).
	Score int
}

// RiskGate performs pre-trade risk assessment on an order.
type RiskGate interface {
	Check(ctx context.Context, o *domain.Order) (RiskDecision, error)
}

// Venue routes an accepted order to an execution venue.
type Venue interface {
	Route(ctx context.Context, o *domain.Order) error
}

// PositionView is a read-model row describing an account's net holding.
type PositionView struct {
	AccountID string
	Symbol    string
	NetQty    string
	AvgPrice  string
}

// PositionReadModel serves the CQRS query side.
type PositionReadModel interface {
	Positions(ctx context.Context, accountID string) ([]PositionView, error)
}
