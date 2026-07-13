// Package memory provides in-memory adapter implementations of the ports. They
// are production-shaped (sharded for concurrency, optimistic concurrency,
// idempotent creation) yet dependency-free, so a Postgres/Kafka adapter can be
// swapped in without touching the core.
package memory

import (
	"context"
	"hash/fnv"
	"sort"
	"sync"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/domain"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/ports"
)

// orderShard is one partition of the order store, independently locked to
// reduce contention under concurrent load.
type orderShard struct {
	mu     sync.RWMutex
	orders map[string]domain.OrderSnapshot
}

// OrderRepository is a sharded, concurrency-safe in-memory order store.
type OrderRepository struct {
	shards []*orderShard
	mask   uint64

	idemMu sync.RWMutex
	idem   map[string]string // idempotency key -> order id
}

// NewOrderRepository builds a repository partitioned into the next power-of-two
// number of shards >= requested (min 1).
func NewOrderRepository(requestedShards int) *OrderRepository {
	n := nextPow2(requestedShards)
	shards := make([]*orderShard, n)
	for i := range shards {
		shards[i] = &orderShard{orders: make(map[string]domain.OrderSnapshot)}
	}
	return &OrderRepository{
		shards: shards,
		mask:   uint64(n - 1),
		idem:   make(map[string]string),
	}
}

// shardFor selects the shard for an id via FNV-1a hashing.
func (r *OrderRepository) shardFor(id string) *orderShard {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return r.shards[h.Sum64()&r.mask]
}

// Save stores a new order and registers its idempotency key.
func (r *OrderRepository) Save(_ context.Context, o *domain.Order, idempotencyKey string) error {
	snap := o.Snapshot()
	sh := r.shardFor(snap.ID)
	sh.mu.Lock()
	if _, exists := sh.orders[snap.ID]; exists {
		sh.mu.Unlock()
		return ports.ErrDuplicate
	}
	sh.orders[snap.ID] = snap
	sh.mu.Unlock()
	if idempotencyKey != "" {
		r.idemMu.Lock()
		r.idem[idempotencyKey] = snap.ID
		r.idemMu.Unlock()
	}
	return nil
}

// Update stores a mutated order under optimistic concurrency control.
func (r *OrderRepository) Update(_ context.Context, o *domain.Order, expectedVersion int) error {
	snap := o.Snapshot()
	sh := r.shardFor(snap.ID)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	cur, ok := sh.orders[snap.ID]
	if !ok {
		return ports.ErrNotFound
	}
	if cur.Version != expectedVersion {
		return ports.ErrVersionConflict
	}
	sh.orders[snap.ID] = snap
	return nil
}

// Get returns a rehydrated copy of the order (safe from external mutation).
func (r *OrderRepository) Get(_ context.Context, id string) (*domain.Order, error) {
	sh := r.shardFor(id)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	snap, ok := sh.orders[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return domain.RehydrateOrder(snap), nil
}

// FindByIdempotencyKey returns a previously stored order id for key.
func (r *OrderRepository) FindByIdempotencyKey(_ context.Context, key string) (string, bool) {
	r.idemMu.RLock()
	defer r.idemMu.RUnlock()
	id, ok := r.idem[key]
	return id, ok
}

// List returns up to limit orders, newest first by creation time.
func (r *OrderRepository) List(_ context.Context, limit int) ([]*domain.Order, error) {
	snaps := make([]domain.OrderSnapshot, 0, 64)
	for _, sh := range r.shards {
		sh.mu.RLock()
		for _, s := range sh.orders {
			snaps = append(snaps, s)
		}
		sh.mu.RUnlock()
	}
	sort.Slice(snaps, func(i, j int) bool {
		return snaps[i].CreatedAt.After(snaps[j].CreatedAt)
	})
	if limit > 0 && len(snaps) > limit {
		snaps = snaps[:limit]
	}
	out := make([]*domain.Order, len(snaps))
	for i, s := range snaps {
		out[i] = domain.RehydrateOrder(s)
	}
	return out, nil
}

// nextPow2 returns the smallest power of two >= n (min 1).
func nextPow2(n int) int {
	if n < 1 {
		return 1
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

var _ ports.OrderRepository = (*OrderRepository)(nil)
