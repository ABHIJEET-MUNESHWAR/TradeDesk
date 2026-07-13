package memory

import (
	"context"
	"sync"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/domain"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/ports"
)

// orderMeta is the static context of an order needed to attribute its fills.
type orderMeta struct {
	account string
	symbol  string
	side    domain.Side
}

// position accumulates net holdings and cost basis for one account+symbol.
type position struct {
	netQty    domain.Quantity
	totalCost domain.Money    // sum of |fill notionals|
	totalAbs  domain.Quantity // sum of |fill quantities|
}

// PositionReadModel is a CQRS read model projected asynchronously from the event
// stream. It subscribes to the bus and serves per-account position queries.
type PositionReadModel struct {
	mu        sync.RWMutex
	meta      map[string]orderMeta
	byAccount map[string]map[string]*position
}

// NewPositionReadModel builds an empty read model.
func NewPositionReadModel() *PositionReadModel {
	return &PositionReadModel{
		meta:      make(map[string]orderMeta),
		byAccount: make(map[string]map[string]*position),
	}
}

// Project handles one event; register it with EventBus.SubscribeAll.
func (rm *PositionReadModel) Project(_ context.Context, e domain.Event) {
	switch ev := e.(type) {
	case domain.OrderPlaced:
		rm.mu.Lock()
		rm.meta[ev.AggregateID()] = orderMeta{account: ev.AccountID, symbol: string(ev.Symbol), side: ev.Side}
		rm.mu.Unlock()
	case domain.OrderPartiallyFilled:
		rm.applyFill(ev.AggregateID(), ev.FillQty, ev.FillPrice)
	case domain.OrderFilled:
		rm.applyFill(ev.AggregateID(), ev.FillQty, ev.FillPrice)
	}
}

// applyFill folds a single execution into the account's net position.
func (rm *PositionReadModel) applyFill(orderID string, qty domain.Quantity, px domain.Money) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	meta, ok := rm.meta[orderID]
	if !ok {
		return
	}
	bySym := rm.byAccount[meta.account]
	if bySym == nil {
		bySym = make(map[string]*position)
		rm.byAccount[meta.account] = bySym
	}
	pos := bySym[meta.symbol]
	if pos == nil {
		pos = &position{}
		bySym[meta.symbol] = pos
	}
	signed := qty
	if meta.side == domain.SideSell {
		signed = -qty
	}
	pos.netQty += signed
	if notional, err := px.MulQuantity(qty); err == nil {
		pos.totalCost += notional
		pos.totalAbs += qty
	}
}

// Positions returns the account's holdings, one row per symbol with a non-zero
// or previously-traded position.
func (rm *PositionReadModel) Positions(_ context.Context, accountID string) ([]ports.PositionView, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	bySym := rm.byAccount[accountID]
	views := make([]ports.PositionView, 0, len(bySym))
	for sym, pos := range bySym {
		avg := domain.Money(0)
		if pos.totalAbs > 0 {
			if a, err := pos.totalCost.DivByQuantity(pos.totalAbs); err == nil {
				avg = a
			}
		}
		views = append(views, ports.PositionView{
			AccountID: accountID,
			Symbol:    sym,
			NetQty:    pos.netQty.String(),
			AvgPrice:  avg.String(),
		})
	}
	return views, nil
}

var _ ports.PositionReadModel = (*PositionReadModel)(nil)
