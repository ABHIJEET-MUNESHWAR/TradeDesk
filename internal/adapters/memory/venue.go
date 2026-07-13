package memory

import (
	"context"
	"sync"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/domain"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/ports"
)

// Filler is invoked by the simulated venue to report an execution back to the
// application (wiring place -> route -> fill end-to-end in demos and tests).
type Filler func(ctx context.Context, orderID string, qty domain.Quantity, px domain.Money)

// SimulatedVenue is a deterministic stand-in for an execution venue. It records
// routed orders and can optionally auto-fill them to exercise the full flow.
type SimulatedVenue struct {
	mu        sync.Mutex
	autofill  bool
	marketPx  domain.Money
	filler    Filler
	routed    []string
	failCount int
}

// NewSimulatedVenue builds an idle venue (no auto-fill).
func NewSimulatedVenue() *SimulatedVenue { return &SimulatedVenue{} }

// SetAutofill enables auto-filling routed orders. marketPx is used as the fill
// price for market orders (limit orders fill at their limit price).
func (v *SimulatedVenue) SetAutofill(on bool, marketPx domain.Money, filler Filler) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.autofill = on
	v.marketPx = marketPx
	v.filler = filler
}

// FailNext makes the next n Route calls fail, to exercise retry/circuit logic.
func (v *SimulatedVenue) FailNext(n int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.failCount = n
}

// Route records the order and optionally auto-fills it.
func (v *SimulatedVenue) Route(ctx context.Context, o *domain.Order) error {
	v.mu.Lock()
	if v.failCount > 0 {
		v.failCount--
		v.mu.Unlock()
		return context.DeadlineExceeded
	}
	v.routed = append(v.routed, o.ID())
	autofill, filler, marketPx := v.autofill, v.filler, v.marketPx
	v.mu.Unlock()

	if autofill && filler != nil {
		px := o.LimitPrice()
		if o.OrderType() == domain.OrderTypeMarket {
			px = marketPx
		}
		filler(context.WithoutCancel(ctx), o.ID(), o.RemainingQuantity(), px)
	}
	return nil
}

// RoutedCount returns how many orders were routed (test aid).
func (v *SimulatedVenue) RoutedCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.routed)
}

var _ ports.Venue = (*SimulatedVenue)(nil)
