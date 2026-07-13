package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/domain"
)

func smallLimitOrder(t *testing.T) *domain.Order {
	t.Helper()
	sym, _ := domain.ParseSymbol("AAPL")
	limit, _ := domain.ParseMoney("150.00")
	qty, _ := domain.ParseQuantity("10")
	o, err := domain.NewOrder("ord-1", "acct-1", sym, domain.SideBuy, domain.OrderTypeLimit, domain.TIFDay, limit, qty, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func hugeOrder(t *testing.T) *domain.Order {
	t.Helper()
	sym, _ := domain.ParseSymbol("AAPL")
	qty, _ := domain.ParseQuantity("500000") // exceeds default MaxQuantity (100k)
	o, err := domain.NewOrder("ord-2", "acct-1", sym, domain.SideBuy, domain.OrderTypeMarket, domain.TIFDay, 0, qty, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return o
}

type fakeCompleter struct {
	out string
	err error
}

func (f fakeCompleter) Complete(_ context.Context, _ string) (string, error) {
	return f.out, f.err
}

func TestHeuristicApprovesNormalOrder(t *testing.T) {
	h := NewHeuristicAnalyzer(DefaultThresholds())
	d, err := h.Check(context.Background(), smallLimitOrder(t))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Approved {
		t.Fatalf("expected approval, got %+v", d)
	}
}

func TestHeuristicRejectsHugeQuantity(t *testing.T) {
	h := NewHeuristicAnalyzer(DefaultThresholds())
	d, err := h.Check(context.Background(), hugeOrder(t))
	if err != nil {
		t.Fatal(err)
	}
	if d.Approved {
		t.Fatalf("expected rejection of oversized order, got %+v", d)
	}
	if d.Reason == "" {
		t.Fatal("rejection should carry a reason")
	}
}

func TestLLMCanOnlyRaiseRisk(t *testing.T) {
	th := DefaultThresholds()
	h := NewHeuristicAnalyzer(th)
	// A normally-approved order that the model flags as very risky (99) is rejected.
	l := NewLLMAnalyzer(h, fakeCompleter{out: "99"}, th)
	d, _ := l.Check(context.Background(), smallLimitOrder(t))
	if d.Approved {
		t.Fatal("model-elevated risk should reject the order")
	}
}

func TestLLMCannotLowerBelowHeuristicFloor(t *testing.T) {
	th := DefaultThresholds()
	h := NewHeuristicAnalyzer(th)
	// Heuristic rejects the huge order; a model score of 0 must NOT approve it.
	l := NewLLMAnalyzer(h, fakeCompleter{out: "0"}, th)
	d, _ := l.Check(context.Background(), hugeOrder(t))
	if d.Approved {
		t.Fatal("model must never lower risk below the heuristic floor")
	}
}

func TestLLMFailsSafeToHeuristic(t *testing.T) {
	th := DefaultThresholds()
	h := NewHeuristicAnalyzer(th)
	l := NewLLMAnalyzer(h, fakeCompleter{err: errors.New("model timeout")}, th)
	d, _ := l.Check(context.Background(), smallLimitOrder(t))
	if !d.Approved {
		t.Fatal("on model error the analyser must fall back to the heuristic (approved)")
	}
}

func TestLLMIgnoresGarbageScore(t *testing.T) {
	th := DefaultThresholds()
	h := NewHeuristicAnalyzer(th)
	l := NewLLMAnalyzer(h, fakeCompleter{out: "not-a-number"}, th)
	d, _ := l.Check(context.Background(), smallLimitOrder(t))
	if !d.Approved {
		t.Fatal("unparsable model output should fall back to heuristic")
	}
}
