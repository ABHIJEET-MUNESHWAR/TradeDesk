package memory

import (
	"context"
	"sync"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/domain"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/ports"
)

// Handler consumes a domain event.
type Handler func(ctx context.Context, e domain.Event)

// subscription is one registered consumer with its own ordered delivery queue.
type subscription struct {
	name    string // event-name filter; "" matches all events
	ch      chan domain.Event
	handler Handler
}

// EventBus is an in-process publish/subscribe bus implementing
// ports.EventPublisher. Each subscriber gets a dedicated FIFO queue drained by a
// single worker goroutine, giving in-order, single-writer projection semantics
// (so, e.g., OrderPlaced is always projected before the fills that follow it)
// while isolating a slow consumer from the write path.
type EventBus struct {
	mu         sync.RWMutex
	subs       []*subscription
	inflight   sync.WaitGroup
	closed     bool
	bufferSize int
}

// NewEventBus builds a bus with a default per-subscriber queue depth.
func NewEventBus() *EventBus { return &EventBus{bufferSize: 4096} }

// Subscribe registers a handler for a specific event name and starts its worker.
func (b *EventBus) Subscribe(eventName string, h Handler) {
	sub := &subscription{name: eventName, ch: make(chan domain.Event, b.bufferSize), handler: h}
	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()
	go b.worker(sub)
}

// SubscribeAll registers a handler for every event (wildcard).
func (b *EventBus) SubscribeAll(h Handler) { b.Subscribe("", h) }

// worker drains a subscription's queue in order using a detached context, since
// the request context that produced the event is cancelled once the HTTP
// handler returns.
func (b *EventBus) worker(sub *subscription) {
	for e := range sub.ch {
		sub.handler(context.Background(), e)
		b.inflight.Done()
	}
}

// Publish enqueues events to every matching subscriber in submission order.
func (b *EventBus) Publish(_ context.Context, events ...domain.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, e := range events {
		for _, sub := range b.subs {
			if sub.name != "" && sub.name != e.EventName() {
				continue
			}
			b.inflight.Add(1)
			sub.ch <- e
		}
	}
}

// Wait blocks until all enqueued events have been handled (deterministic tests).
func (b *EventBus) Wait() { b.inflight.Wait() }

// Close drains outstanding events and stops all worker goroutines.
func (b *EventBus) Close() {
	b.Wait()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, sub := range b.subs {
		close(sub.ch)
	}
}

var _ ports.EventPublisher = (*EventBus)(nil)
