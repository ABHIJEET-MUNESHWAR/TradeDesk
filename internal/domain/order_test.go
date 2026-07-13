package domain

import (
	"errors"
	"testing"
	"time"
)

func mustSymbol(t *testing.T, s string) Symbol {
	t.Helper()
	sym, err := ParseSymbol(s)
	if err != nil {
		t.Fatalf("ParseSymbol(%q): %v", s, err)
	}
	return sym
}

func mustMoney(t *testing.T, s string) Money {
	t.Helper()
	m, err := ParseMoney(s)
	if err != nil {
		t.Fatalf("ParseMoney(%q): %v", s, err)
	}
	return m
}

func mustQty(t *testing.T, s string) Quantity {
	t.Helper()
	q, err := ParseQuantity(s)
	if err != nil {
		t.Fatalf("ParseQuantity(%q): %v", s, err)
	}
	return q
}

func newLimitOrder(t *testing.T) *Order {
	t.Helper()
	o, err := NewOrder("ord-1", "acct-1", mustSymbol(t, "AAPL"), SideBuy, OrderTypeLimit, TIFDay, mustMoney(t, "150.00"), mustQty(t, "10"), time.Now())
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	return o
}

func TestParseEnums(t *testing.T) {
	if _, err := ParseSide("buy"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSide("hold"); err == nil {
		t.Fatal("expected invalid side")
	}
	if _, err := ParseOrderType("limit"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseTimeInForce("gtc"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSymbol("toolongsymbol"); err == nil {
		t.Fatal("expected invalid symbol")
	}
	if _, err := ParseSymbol("AA1"); err == nil {
		t.Fatal("expected invalid symbol with digit")
	}
}

func TestNewOrderValidation(t *testing.T) {
	sym := mustSymbol(t, "AAPL")
	now := time.Now()
	// limit order without price
	if _, err := NewOrder("id", "acct", sym, SideBuy, OrderTypeLimit, TIFDay, 0, mustQty(t, "1"), now); !errors.Is(err, ErrLimitPriceRequired) {
		t.Fatalf("want ErrLimitPriceRequired, got %v", err)
	}
	// market order with price
	if _, err := NewOrder("id", "acct", sym, SideBuy, OrderTypeMarket, TIFDay, mustMoney(t, "1"), mustQty(t, "1"), now); !errors.Is(err, ErrMarketPriceSet) {
		t.Fatalf("want ErrMarketPriceSet, got %v", err)
	}
	// zero quantity
	if _, err := NewOrder("id", "acct", sym, SideBuy, OrderTypeMarket, TIFDay, 0, 0, now); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("want ErrInvalidQuantity, got %v", err)
	}
	// empty account
	if _, err := NewOrder("id", "", sym, SideBuy, OrderTypeMarket, TIFDay, 0, mustQty(t, "1"), now); !errors.Is(err, ErrEmptyAccount) {
		t.Fatalf("want ErrEmptyAccount, got %v", err)
	}
}

func TestOrderLifecycleFillVWAP(t *testing.T) {
	o := newLimitOrder(t)
	if o.Status() != StatusNew {
		t.Fatalf("want NEW, got %s", o.Status())
	}
	if err := o.Accept(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := o.Fill(mustQty(t, "5"), mustMoney(t, "150.00"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if o.Status() != StatusPartiallyFilled {
		t.Fatalf("want PARTIALLY_FILLED, got %s", o.Status())
	}
	if err := o.Fill(mustQty(t, "5"), mustMoney(t, "152.00"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if o.Status() != StatusFilled {
		t.Fatalf("want FILLED, got %s", o.Status())
	}
	// VWAP = (150*5 + 152*5) / 10 = 151.00
	if got := o.AvgFillPrice().String(); got != "151.0000" {
		t.Fatalf("VWAP=%s want 151.0000", got)
	}
	if !o.RemainingQuantity().IsZero() {
		t.Fatalf("remaining should be zero, got %s", o.RemainingQuantity())
	}
}

func TestOrderOverfillRejected(t *testing.T) {
	o := newLimitOrder(t)
	_ = o.Accept(time.Now())
	if err := o.Fill(mustQty(t, "11"), mustMoney(t, "150.00"), time.Now()); !errors.Is(err, ErrOverfill) {
		t.Fatalf("want ErrOverfill, got %v", err)
	}
}

func TestIllegalTransitions(t *testing.T) {
	o := newLimitOrder(t)
	// cannot fill a NEW (unaccepted) order
	if err := o.Fill(mustQty(t, "5"), mustMoney(t, "150.00"), time.Now()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}
	// fully fill then attempt cancel
	_ = o.Accept(time.Now())
	_ = o.Fill(mustQty(t, "10"), mustMoney(t, "150.00"), time.Now())
	if !o.Status().IsTerminal() {
		t.Fatal("filled order should be terminal")
	}
	if err := o.Cancel(time.Now()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition on cancel of filled, got %v", err)
	}
}

func TestRejectAndCancelPaths(t *testing.T) {
	o := newLimitOrder(t)
	if err := o.Reject("too big", time.Now()); err != nil {
		t.Fatal(err)
	}
	if o.Status() != StatusRejected {
		t.Fatalf("want REJECTED, got %s", o.Status())
	}
	o2 := newLimitOrder(t)
	_ = o2.Accept(time.Now())
	if err := o2.Cancel(time.Now()); err != nil {
		t.Fatal(err)
	}
	if o2.Status() != StatusCanceled {
		t.Fatalf("want CANCELED, got %s", o2.Status())
	}
}

func TestPullEventsExactlyOnce(t *testing.T) {
	o := newLimitOrder(t)
	_ = o.Accept(time.Now())
	events := o.PullEvents()
	if len(events) != 2 {
		t.Fatalf("want 2 events (placed+accepted), got %d", len(events))
	}
	if _, ok := events[0].(OrderPlaced); !ok {
		t.Fatalf("first event should be OrderPlaced, got %T", events[0])
	}
	if again := o.PullEvents(); again != nil {
		t.Fatalf("second pull should be empty, got %d", len(again))
	}
}

func TestSnapshotRehydrate(t *testing.T) {
	o := newLimitOrder(t)
	_ = o.Accept(time.Now())
	_ = o.Fill(mustQty(t, "4"), mustMoney(t, "150.00"), time.Now())
	snap := o.Snapshot()
	clone := RehydrateOrder(snap)
	if clone.Status() != o.Status() || clone.Version() != o.Version() {
		t.Fatalf("rehydrate mismatch: %s v%d vs %s v%d", clone.Status(), clone.Version(), o.Status(), o.Version())
	}
	if clone.FilledQuantity() != o.FilledQuantity() {
		t.Fatal("filled quantity mismatch after rehydrate")
	}
	// rehydrated order carries no buffered events
	if evs := clone.PullEvents(); evs != nil {
		t.Fatalf("rehydrated order should have no events, got %d", len(evs))
	}
}
