package domain

import (
	"strings"
	"time"
)

// Order is the aggregate root of the trading domain. All state changes go
// through methods that enforce the lifecycle state machine and emit events, so
// an order can never reach an inconsistent state. Fields are unexported to keep
// the invariants encapsulated; persistence uses Snapshot / RehydrateOrder.
type Order struct {
	id         string
	accountID  string
	symbol     Symbol
	side       Side
	orderType  OrderType
	tif        TimeInForce
	limitPrice Money
	quantity   Quantity
	filledQty  Quantity
	avgFillPx  Money
	status     OrderStatus
	version    int
	createdAt  time.Time
	updatedAt  time.Time
	events     []Event
}

// NewOrder validates its inputs and returns an order in the NEW state with an
// OrderPlaced event buffered. `now` and `id` are injected to keep the domain
// pure and deterministic.
func NewOrder(id, accountID string, symbol Symbol, side Side, ot OrderType, tif TimeInForce, limitPrice Money, quantity Quantity, now time.Time) (*Order, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrEmptyID
	}
	if strings.TrimSpace(accountID) == "" {
		return nil, ErrEmptyAccount
	}
	if symbol == "" {
		return nil, ErrInvalidSymbol
	}
	if side != SideBuy && side != SideSell {
		return nil, ErrInvalidSide
	}
	if ot != OrderTypeMarket && ot != OrderTypeLimit {
		return nil, ErrInvalidOrderType
	}
	switch tif {
	case TIFDay, TIFGTC, TIFIOC, TIFFOK:
	default:
		return nil, ErrInvalidTIF
	}
	if !quantity.IsPositive() {
		return nil, ErrInvalidQuantity
	}
	if ot == OrderTypeLimit && limitPrice <= 0 {
		return nil, ErrLimitPriceRequired
	}
	if ot == OrderTypeMarket && limitPrice != 0 {
		return nil, ErrMarketPriceSet
	}
	o := &Order{
		id:         id,
		accountID:  accountID,
		symbol:     symbol,
		side:       side,
		orderType:  ot,
		tif:        tif,
		limitPrice: limitPrice,
		quantity:   quantity,
		status:     StatusNew,
		createdAt:  now,
		updatedAt:  now,
	}
	o.raise(OrderPlaced{
		baseEvent:  baseEvent{orderID: id, at: now},
		AccountID:  accountID,
		Symbol:     symbol,
		Side:       side,
		OrderType:  ot,
		TIF:        tif,
		Quantity:   quantity,
		LimitPrice: limitPrice,
	})
	return o, nil
}

// raise buffers an emitted event.
func (o *Order) raise(e Event) { o.events = append(o.events, e) }

// PullEvents returns and clears the buffered events, giving the caller
// exactly-once ownership for publication.
func (o *Order) PullEvents() []Event {
	e := o.events
	o.events = nil
	return e
}

// transition enforces the state machine and bumps the version + timestamp.
func (o *Order) transition(to OrderStatus, now time.Time) error {
	if !o.status.canTransitionTo(to) {
		return ErrInvalidTransition
	}
	o.status = to
	o.updatedAt = now
	o.version++
	return nil
}

// Accept moves a NEW order to ACCEPTED after passing pre-trade risk.
func (o *Order) Accept(now time.Time) error {
	if err := o.transition(StatusAccepted, now); err != nil {
		return err
	}
	o.raise(OrderAccepted{baseEvent{o.id, now}})
	return nil
}

// Reject moves a NEW order to REJECTED with a reason.
func (o *Order) Reject(reason string, now time.Time) error {
	if err := o.transition(StatusRejected, now); err != nil {
		return err
	}
	o.raise(OrderRejected{baseEvent{o.id, now}, reason})
	return nil
}

// Cancel moves a working order to CANCELED.
func (o *Order) Cancel(now time.Time) error {
	if err := o.transition(StatusCanceled, now); err != nil {
		return err
	}
	o.raise(OrderCanceled{baseEvent{o.id, now}})
	return nil
}

// Expire moves a working order to EXPIRED by its time-in-force.
func (o *Order) Expire(now time.Time) error {
	if err := o.transition(StatusExpired, now); err != nil {
		return err
	}
	o.raise(OrderExpired{baseEvent{o.id, now}})
	return nil
}

// Fill applies an execution of qty shares at px, updating the volume-weighted
// average fill price and moving to PARTIALLY_FILLED or FILLED. Over-fills and
// fills on a non-working order are rejected.
func (o *Order) Fill(qty Quantity, px Money, now time.Time) error {
	if o.status != StatusAccepted && o.status != StatusPartiallyFilled {
		return ErrInvalidTransition
	}
	if !qty.IsPositive() {
		return ErrInvalidQuantity
	}
	if px <= 0 {
		return ErrFillPrice
	}
	newFilled, err := o.filledQty.Add(qty)
	if err != nil {
		return err
	}
	if newFilled > o.quantity {
		return ErrOverfill
	}
	oldNotional, err := o.avgFillPx.MulQuantity(o.filledQty)
	if err != nil {
		return err
	}
	addNotional, err := px.MulQuantity(qty)
	if err != nil {
		return err
	}
	totalNotional, err := oldNotional.Add(addNotional)
	if err != nil {
		return err
	}
	newAvg, err := totalNotional.DivByQuantity(newFilled)
	if err != nil {
		return err
	}
	o.filledQty = newFilled
	o.avgFillPx = newAvg
	if newFilled == o.quantity {
		_ = o.transition(StatusFilled, now)
		o.raise(OrderFilled{baseEvent{o.id, now}, qty, px, o.avgFillPx})
	} else {
		_ = o.transition(StatusPartiallyFilled, now)
		o.raise(OrderPartiallyFilled{baseEvent{o.id, now}, qty, px, o.filledQty, o.avgFillPx})
	}
	return nil
}

// ID returns the order id.
func (o *Order) ID() string { return o.id }

// AccountID returns the owning account id.
func (o *Order) AccountID() string { return o.accountID }

// Symbol returns the ticker.
func (o *Order) Symbol() Symbol { return o.symbol }

// Side returns the order side.
func (o *Order) Side() Side { return o.side }

// OrderType returns the execution instruction.
func (o *Order) OrderType() OrderType { return o.orderType }

// TIF returns the time-in-force.
func (o *Order) TIF() TimeInForce { return o.tif }

// LimitPrice returns the limit price (zero for market orders).
func (o *Order) LimitPrice() Money { return o.limitPrice }

// Quantity returns the ordered quantity.
func (o *Order) Quantity() Quantity { return o.quantity }

// FilledQuantity returns the cumulative filled quantity.
func (o *Order) FilledQuantity() Quantity { return o.filledQty }

// RemainingQuantity returns the unfilled quantity.
func (o *Order) RemainingQuantity() Quantity { return o.quantity - o.filledQty }

// AvgFillPrice returns the volume-weighted average fill price.
func (o *Order) AvgFillPrice() Money { return o.avgFillPx }

// Status returns the current lifecycle status.
func (o *Order) Status() OrderStatus { return o.status }

// Version returns the optimistic-concurrency version.
func (o *Order) Version() int { return o.version }

// CreatedAt returns the creation timestamp.
func (o *Order) CreatedAt() time.Time { return o.createdAt }

// UpdatedAt returns the last-update timestamp.
func (o *Order) UpdatedAt() time.Time { return o.updatedAt }

// OrderSnapshot is a flat, copyable representation of an order used for
// persistence and safe cloning (it excludes the pending-event buffer).
type OrderSnapshot struct {
	ID         string
	AccountID  string
	Symbol     Symbol
	Side       Side
	OrderType  OrderType
	TIF        TimeInForce
	LimitPrice Money
	Quantity   Quantity
	FilledQty  Quantity
	AvgFillPx  Money
	Status     OrderStatus
	Version    int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Snapshot returns a copyable snapshot of the order's state.
func (o *Order) Snapshot() OrderSnapshot {
	return OrderSnapshot{
		ID:         o.id,
		AccountID:  o.accountID,
		Symbol:     o.symbol,
		Side:       o.side,
		OrderType:  o.orderType,
		TIF:        o.tif,
		LimitPrice: o.limitPrice,
		Quantity:   o.quantity,
		FilledQty:  o.filledQty,
		AvgFillPx:  o.avgFillPx,
		Status:     o.status,
		Version:    o.version,
		CreatedAt:  o.createdAt,
		UpdatedAt:  o.updatedAt,
	}
}

// RehydrateOrder reconstructs an order from a snapshot (no events buffered).
func RehydrateOrder(s OrderSnapshot) *Order {
	return &Order{
		id:         s.ID,
		accountID:  s.AccountID,
		symbol:     s.Symbol,
		side:       s.Side,
		orderType:  s.OrderType,
		tif:        s.TIF,
		limitPrice: s.LimitPrice,
		quantity:   s.Quantity,
		filledQty:  s.FilledQty,
		avgFillPx:  s.AvgFillPx,
		status:     s.Status,
		version:    s.Version,
		createdAt:  s.CreatedAt,
		updatedAt:  s.UpdatedAt,
	}
}
