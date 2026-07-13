package memory

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/domain"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/ports"
)

func mustM(t *testing.T, s string) domain.Money {
	t.Helper()
	m, err := domain.ParseMoney(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func mustQ(t *testing.T, s string) domain.Quantity {
	t.Helper()
	q, err := domain.ParseQuantity(s)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func testOrder(t *testing.T, id string) *domain.Order {
	t.Helper()
	sym, _ := domain.ParseSymbol("AAPL")
	o, err := domain.NewOrder(id, "acct-1", sym, domain.SideBuy, domain.OrderTypeLimit, domain.TIFDay, mustM(t, "150.00"), mustQ(t, "10"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestRepositorySaveGet(t *testing.T) {
	r := NewOrderRepository(8)
	if err := r.Save(context.Background(), testOrder(t, "ord-1"), "idem-1"); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(context.Background(), "ord-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID() != "ord-1" {
		t.Fatalf("id=%s", got.ID())
	}
	if _, err := r.Get(context.Background(), "missing"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestRepositoryDuplicateSave(t *testing.T) {
	r := NewOrderRepository(4)
	_ = r.Save(context.Background(), testOrder(t, "ord-1"), "")
	if err := r.Save(context.Background(), testOrder(t, "ord-1"), ""); !errors.Is(err, ports.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestRepositoryIdempotency(t *testing.T) {
	r := NewOrderRepository(4)
	_ = r.Save(context.Background(), testOrder(t, "ord-1"), "idem-1")
	id, ok := r.FindByIdempotencyKey(context.Background(), "idem-1")
	if !ok || id != "ord-1" {
		t.Fatalf("idem lookup failed: %q %v", id, ok)
	}
	if _, ok := r.FindByIdempotencyKey(context.Background(), "nope"); ok {
		t.Fatal("unexpected idem hit")
	}
}

func TestRepositoryOptimisticConcurrency(t *testing.T) {
	r := NewOrderRepository(4)
	o := testOrder(t, "ord-1")
	_ = r.Save(context.Background(), o, "") // stored at version 0
	_ = o.Accept(time.Now())                // version 1
	if err := r.Update(context.Background(), o, 0); err != nil {
		t.Fatalf("update v0: %v", err)
	}
	stale, _ := r.Get(context.Background(), "ord-1") // version 1
	_ = stale.Cancel(time.Now())                     // version 2
	if err := r.Update(context.Background(), stale, 0); !errors.Is(err, ports.ErrVersionConflict) {
		t.Fatalf("want ErrVersionConflict, got %v", err)
	}
}

func TestRepositoryListNewestFirst(t *testing.T) {
	r := NewOrderRepository(8)
	for _, id := range []string{"ord-1", "ord-2", "ord-3"} {
		_ = r.Save(context.Background(), testOrder(t, id), "")
		time.Sleep(time.Millisecond)
	}
	list, err := r.List(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2, got %d", len(list))
	}
}

func TestEventBusOrderedDelivery(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()
	var mu sync.Mutex
	var got []string
	bus.SubscribeAll(func(_ context.Context, e domain.Event) {
		mu.Lock()
		got = append(got, e.EventName())
		mu.Unlock()
	})
	o := testOrder(t, "ord-1")
	_ = o.Accept(time.Now())
	bus.Publish(context.Background(), o.PullEvents()...)
	bus.Wait()
	if len(got) != 2 || got[0] != "OrderPlaced" || got[1] != "OrderAccepted" {
		t.Fatalf("ordering wrong: %v", got)
	}
}

func TestEventBusNameFilter(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()
	var mu sync.Mutex
	count := 0
	bus.Subscribe("OrderAccepted", func(_ context.Context, _ domain.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	o := testOrder(t, "ord-1")
	_ = o.Accept(time.Now())
	bus.Publish(context.Background(), o.PullEvents()...)
	bus.Wait()
	if count != 1 {
		t.Fatalf("filtered handler fired %d times", count)
	}
}

func TestPositionReadModelProjection(t *testing.T) {
	rm := NewPositionReadModel()
	bus := NewEventBus()
	defer bus.Close()
	bus.SubscribeAll(rm.Project)
	o := testOrder(t, "ord-1")
	_ = o.Accept(time.Now())
	_ = o.Fill(mustQ(t, "10"), mustM(t, "150.00"), time.Now())
	bus.Publish(context.Background(), o.PullEvents()...)
	bus.Wait()
	pos, _ := rm.Positions(context.Background(), "acct-1")
	if len(pos) != 1 {
		t.Fatalf("want 1 position, got %d", len(pos))
	}
	if pos[0].Symbol != "AAPL" || pos[0].NetQty != "10.000" || pos[0].AvgPrice != "150.0000" {
		t.Fatalf("unexpected position: %+v", pos[0])
	}
}

func TestSimulatedVenueAutofill(t *testing.T) {
	v := NewSimulatedVenue()
	var gotID string
	var gotQty domain.Quantity
	v.SetAutofill(true, mustM(t, "150.00"), func(_ context.Context, orderID string, qty domain.Quantity, _ domain.Money) {
		gotID = orderID
		gotQty = qty
	})
	o := testOrder(t, "ord-1")
	_ = o.Accept(time.Now())
	if err := v.Route(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if gotID != "ord-1" || gotQty != mustQ(t, "10") {
		t.Fatalf("autofill wrong: %s %s", gotID, gotQty)
	}
	if v.RoutedCount() != 1 {
		t.Fatalf("route count=%d", v.RoutedCount())
	}
}

func TestSimulatedVenueFailNext(t *testing.T) {
	v := NewSimulatedVenue()
	v.FailNext(1)
	o := testOrder(t, "ord-1")
	if err := v.Route(context.Background(), o); err == nil {
		t.Fatal("expected first route to fail")
	}
	if err := v.Route(context.Background(), o); err != nil {
		t.Fatalf("second route should succeed: %v", err)
	}
}
