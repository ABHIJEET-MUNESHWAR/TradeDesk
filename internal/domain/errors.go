package domain

import "errors"

// Domain validation and state-machine errors. Returning typed sentinels lets
// the application layer map failures to precise API responses.
var (
	ErrEmptyID            = errors.New("domain: order id required")
	ErrEmptyAccount       = errors.New("domain: account id required")
	ErrInvalidSide        = errors.New("domain: invalid order side")
	ErrInvalidOrderType   = errors.New("domain: invalid order type")
	ErrInvalidTIF         = errors.New("domain: invalid time-in-force")
	ErrInvalidSymbol      = errors.New("domain: invalid symbol")
	ErrInvalidQuantity    = errors.New("domain: quantity must be positive")
	ErrLimitPriceRequired = errors.New("domain: limit order requires a positive limit price")
	ErrMarketPriceSet     = errors.New("domain: market order must not carry a limit price")
	ErrInvalidTransition  = errors.New("domain: illegal status transition")
	ErrOverfill           = errors.New("domain: fill exceeds remaining quantity")
	ErrFillPrice          = errors.New("domain: fill price must be positive")
)
