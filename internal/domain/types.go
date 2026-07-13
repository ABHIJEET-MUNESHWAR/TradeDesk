package domain

import "strings"

// Side is the direction of an order.
type Side string

// Order sides.
const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

// ParseSide validates and normalises a side string.
func ParseSide(s string) (Side, error) {
	switch Side(strings.ToUpper(strings.TrimSpace(s))) {
	case SideBuy:
		return SideBuy, nil
	case SideSell:
		return SideSell, nil
	default:
		return "", ErrInvalidSide
	}
}

// OrderType is the execution instruction of an order.
type OrderType string

// Order types.
const (
	OrderTypeMarket OrderType = "MARKET"
	OrderTypeLimit  OrderType = "LIMIT"
)

// ParseOrderType validates and normalises an order-type string.
func ParseOrderType(s string) (OrderType, error) {
	switch OrderType(strings.ToUpper(strings.TrimSpace(s))) {
	case OrderTypeMarket:
		return OrderTypeMarket, nil
	case OrderTypeLimit:
		return OrderTypeLimit, nil
	default:
		return "", ErrInvalidOrderType
	}
}

// TimeInForce controls how long an order remains active.
type TimeInForce string

// Time-in-force policies.
const (
	TIFDay TimeInForce = "DAY"
	TIFGTC TimeInForce = "GTC"
	TIFIOC TimeInForce = "IOC"
	TIFFOK TimeInForce = "FOK"
)

// ParseTimeInForce validates and normalises a time-in-force string.
func ParseTimeInForce(s string) (TimeInForce, error) {
	switch TimeInForce(strings.ToUpper(strings.TrimSpace(s))) {
	case TIFDay:
		return TIFDay, nil
	case TIFGTC:
		return TIFGTC, nil
	case TIFIOC:
		return TIFIOC, nil
	case TIFFOK:
		return TIFFOK, nil
	default:
		return "", ErrInvalidTIF
	}
}

// Symbol is a validated ticker (1-8 uppercase letters).
type Symbol string

// ParseSymbol validates and normalises a ticker symbol.
func ParseSymbol(s string) (Symbol, error) {
	u := strings.ToUpper(strings.TrimSpace(s))
	if len(u) < 1 || len(u) > 8 {
		return "", ErrInvalidSymbol
	}
	for _, r := range u {
		if r < 'A' || r > 'Z' {
			return "", ErrInvalidSymbol
		}
	}
	return Symbol(u), nil
}

// OrderStatus is a state in the order lifecycle.
type OrderStatus string

// Order lifecycle states.
const (
	StatusNew             OrderStatus = "NEW"
	StatusAccepted        OrderStatus = "ACCEPTED"
	StatusPartiallyFilled OrderStatus = "PARTIALLY_FILLED"
	StatusFilled          OrderStatus = "FILLED"
	StatusCanceled        OrderStatus = "CANCELED"
	StatusRejected        OrderStatus = "REJECTED"
	StatusExpired         OrderStatus = "EXPIRED"
)

// legalTransitions is the adjacency map of the order state machine. Encoding it
// as data (rather than scattered if-statements) makes illegal transitions
// impossible to reach and trivial to audit.
var legalTransitions = map[OrderStatus]map[OrderStatus]bool{
	StatusNew:             {StatusAccepted: true, StatusRejected: true},
	StatusAccepted:        {StatusPartiallyFilled: true, StatusFilled: true, StatusCanceled: true, StatusExpired: true},
	StatusPartiallyFilled: {StatusPartiallyFilled: true, StatusFilled: true, StatusCanceled: true, StatusExpired: true},
	StatusFilled:          {},
	StatusCanceled:        {},
	StatusRejected:        {},
	StatusExpired:         {},
}

// canTransitionTo reports whether moving to `to` is a legal transition.
func (s OrderStatus) canTransitionTo(to OrderStatus) bool {
	return legalTransitions[s][to]
}

// IsTerminal reports whether the status admits no further transitions.
func (s OrderStatus) IsTerminal() bool {
	return len(legalTransitions[s]) == 0
}
