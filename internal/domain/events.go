package domain

import "time"

// Event is a domain event emitted by the Order aggregate. Events are the source
// of truth on the write side and drive read-model projections (CQRS).
type Event interface {
	// AggregateID is the id of the order that emitted the event.
	AggregateID() string
	// EventName is the stable wire name used for routing and serialisation.
	EventName() string
	// OccurredAt is the event timestamp.
	OccurredAt() time.Time
}

// baseEvent carries fields common to every event.
type baseEvent struct {
	orderID string
	at      time.Time
}

func (b baseEvent) AggregateID() string   { return b.orderID }
func (b baseEvent) OccurredAt() time.Time { return b.at }

// OrderPlaced is emitted when a new order is created.
type OrderPlaced struct {
	baseEvent
	AccountID  string
	Symbol     Symbol
	Side       Side
	OrderType  OrderType
	TIF        TimeInForce
	Quantity   Quantity
	LimitPrice Money
}

// EventName implements Event.
func (OrderPlaced) EventName() string { return "OrderPlaced" }

// OrderAccepted is emitted when an order passes pre-trade risk.
type OrderAccepted struct{ baseEvent }

// EventName implements Event.
func (OrderAccepted) EventName() string { return "OrderAccepted" }

// OrderRejected is emitted when an order fails pre-trade risk.
type OrderRejected struct {
	baseEvent
	Reason string
}

// EventName implements Event.
func (OrderRejected) EventName() string { return "OrderRejected" }

// OrderPartiallyFilled is emitted when an order receives a partial fill.
type OrderPartiallyFilled struct {
	baseEvent
	FillQty       Quantity
	FillPrice     Money
	CumulativeQty Quantity
	AvgFillPrice  Money
}

// EventName implements Event.
func (OrderPartiallyFilled) EventName() string { return "OrderPartiallyFilled" }

// OrderFilled is emitted when an order is completely filled.
type OrderFilled struct {
	baseEvent
	FillQty      Quantity
	FillPrice    Money
	AvgFillPrice Money
}

// EventName implements Event.
func (OrderFilled) EventName() string { return "OrderFilled" }

// OrderCanceled is emitted when an order is cancelled.
type OrderCanceled struct{ baseEvent }

// EventName implements Event.
func (OrderCanceled) EventName() string { return "OrderCanceled" }

// OrderExpired is emitted when an order expires by its time-in-force.
type OrderExpired struct{ baseEvent }

// EventName implements Event.
func (OrderExpired) EventName() string { return "OrderExpired" }
