package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/adapters/ai"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/adapters/memory"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/adapters/system"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/app"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/observability"
)

func newTestService(t *testing.T) (*app.Service, *memory.EventBus, *memory.PositionReadModel) {
	t.Helper()
	reg := prometheus.NewRegistry()
	metrics := observability.NewMetrics(reg)
	repo := memory.NewOrderRepository(8)
	bus := memory.NewEventBus()
	rm := memory.NewPositionReadModel()
	bus.SubscribeAll(rm.Project)
	venue := memory.NewSimulatedVenue()
	risk := ai.NewHeuristicAnalyzer(ai.DefaultThresholds())
	svc := app.NewService(app.Deps{
		Repo:      repo,
		Publisher: bus,
		Risk:      risk,
		Venue:     venue,
		Positions: rm,
		Clock:     system.Clock{},
		IDs:       system.IDGenerator{},
		Logger:    observability.NewLogger("error"),
		Metrics:   metrics,
	}, app.Config{
		RiskTimeout:     time.Second,
		RouteTimeout:    time.Second,
		RetryAttempts:   3,
		RetryBase:       time.Millisecond,
		RetryMax:        10 * time.Millisecond,
		RateLimitRPS:    1000,
		RateLimitBurst:  1000,
		BreakerFailures: 5,
		BreakerCooldown: time.Second,
	})
	return svc, bus, rm
}

func validLimit() app.PlaceOrderCommand {
	return app.PlaceOrderCommand{
		AccountID:  "acct-1",
		Symbol:     "AAPL",
		Side:       "BUY",
		OrderType:  "LIMIT",
		TIF:        "DAY",
		Quantity:   "10",
		LimitPrice: "150.00",
	}
}

func TestPlaceOrderAcceptedFilledAndProjected(t *testing.T) {
	svc, bus, rm := newTestService(t)
	defer bus.Close()

	cmd := validLimit()
	cmd.IdempotencyKey = "k1"
	res, err := svc.PlaceOrder(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "ACCEPTED" || res.OrderID == "" {
		t.Fatalf("unexpected result: %+v", res)
	}

	// Idempotent replay returns the same order without creating a new one.
	res2, err := svc.PlaceOrder(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Idempotent || res2.OrderID != res.OrderID {
		t.Fatalf("idempotency failed: %+v", res2)
	}

	if err := svc.ApplyFill(context.Background(), app.ApplyFillCommand{OrderID: res.OrderID, Quantity: "10", Price: "150.00"}); err != nil {
		t.Fatal(err)
	}
	view, err := svc.GetOrder(context.Background(), res.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != "FILLED" {
		t.Fatalf("status=%s", view.Status)
	}

	bus.Wait()
	pos, _ := svc.Positions(context.Background(), "acct-1")
	if len(pos) != 1 || pos[0].NetQty != "10.000" {
		t.Fatalf("positions=%+v", pos)
	}
	_ = rm
}

func TestPlaceOrderRejectedByRisk(t *testing.T) {
	svc, bus, _ := newTestService(t)
	defer bus.Close()
	res, err := svc.PlaceOrder(context.Background(), app.PlaceOrderCommand{
		AccountID: "acct-1", Symbol: "AAPL", Side: "BUY", OrderType: "MARKET", TIF: "DAY", Quantity: "500000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "REJECTED" {
		t.Fatalf("want REJECTED, got %s", res.Status)
	}
}

func TestPlaceOrderValidationError(t *testing.T) {
	svc, bus, _ := newTestService(t)
	defer bus.Close()
	// LIMIT order without a price is invalid.
	_, err := svc.PlaceOrder(context.Background(), app.PlaceOrderCommand{
		AccountID: "acct-1", Symbol: "AAPL", Side: "BUY", OrderType: "LIMIT", TIF: "DAY", Quantity: "10",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCancelOrder(t *testing.T) {
	svc, bus, _ := newTestService(t)
	defer bus.Close()
	res, _ := svc.PlaceOrder(context.Background(), validLimit())
	if err := svc.CancelOrder(context.Background(), app.CancelOrderCommand{OrderID: res.OrderID}); err != nil {
		t.Fatal(err)
	}
	view, _ := svc.GetOrder(context.Background(), res.OrderID)
	if view.Status != "CANCELED" {
		t.Fatalf("status=%s", view.Status)
	}
}

func TestApplyFillUnknownOrder(t *testing.T) {
	svc, bus, _ := newTestService(t)
	defer bus.Close()
	if err := svc.ApplyFill(context.Background(), app.ApplyFillCommand{OrderID: "nope", Quantity: "1", Price: "1"}); err == nil {
		t.Fatal("expected error for unknown order")
	}
}

func TestListOrders(t *testing.T) {
	svc, bus, _ := newTestService(t)
	defer bus.Close()
	for i := 0; i < 3; i++ {
		if _, err := svc.PlaceOrder(context.Background(), validLimit()); err != nil {
			t.Fatal(err)
		}
	}
	orders, err := svc.ListOrders(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 3 {
		t.Fatalf("want 3, got %d", len(orders))
	}
}
