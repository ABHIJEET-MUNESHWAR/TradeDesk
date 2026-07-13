package app

import (
	"time"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/domain"
)

// PlaceOrderCommand is the write-side input to place a new order.
type PlaceOrderCommand struct {
	AccountID      string
	Symbol         string
	Side           string
	OrderType      string
	TIF            string
	Quantity       string
	LimitPrice     string
	IdempotencyKey string
}

// PlaceOrderResult is the outcome of a PlaceOrder command.
type PlaceOrderResult struct {
	OrderID    string
	Status     string
	Idempotent bool
}

// CancelOrderCommand cancels a working order.
type CancelOrderCommand struct {
	OrderID string
}

// ApplyFillCommand records an execution against an order.
type ApplyFillCommand struct {
	OrderID  string
	Quantity string
	Price    string
}

// OrderView is the read-side projection of an order returned by queries.
type OrderView struct {
	ID           string
	AccountID    string
	Symbol       string
	Side         string
	OrderType    string
	TIF          string
	LimitPrice   string
	Quantity     string
	FilledQty    string
	AvgFillPrice string
	Status       string
	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// viewFromOrder maps a domain order to its API view.
func viewFromOrder(o *domain.Order) OrderView {
	return OrderView{
		ID:           o.ID(),
		AccountID:    o.AccountID(),
		Symbol:       string(o.Symbol()),
		Side:         string(o.Side()),
		OrderType:    string(o.OrderType()),
		TIF:          string(o.TIF()),
		LimitPrice:   o.LimitPrice().String(),
		Quantity:     o.Quantity().String(),
		FilledQty:    o.FilledQuantity().String(),
		AvgFillPrice: o.AvgFillPrice().String(),
		Status:       string(o.Status()),
		Version:      o.Version(),
		CreatedAt:    o.CreatedAt(),
		UpdatedAt:    o.UpdatedAt(),
	}
}
