package domain

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
)

const (
	// moneyScale is the number of Money minor units per whole USD (4 dp).
	moneyScale = 10000
	// quantityScale is the number of Quantity minor units per whole share (3 dp).
	quantityScale = 1000
)

// Sentinel arithmetic / parsing errors.
var (
	ErrOverflow      = errors.New("domain: arithmetic overflow")
	ErrParseMoney    = errors.New("domain: invalid money literal")
	ErrParseQuantity = errors.New("domain: invalid quantity literal")
)

// Money is a signed fixed-point USD amount stored as 1/10000-dollar minor units.
// Keeping all money math on integers avoids floating-point drift on the hot path
// so ledger amounts always reconcile.
type Money int64

// Quantity is a share amount stored as milli-share (1/1000) minor units, which
// supports fractional-share trading without floats.
type Quantity int64

// ParseMoney converts a decimal string such as "123.45" into Money.
func ParseMoney(s string) (Money, error) {
	v, err := parseFixed(s, moneyScale)
	if err != nil {
		return 0, ErrParseMoney
	}
	return Money(v), nil
}

// ParseQuantity converts a decimal string such as "1.5" into Quantity.
func ParseQuantity(s string) (Quantity, error) {
	v, err := parseFixed(s, quantityScale)
	if err != nil {
		return 0, ErrParseQuantity
	}
	return Quantity(v), nil
}

// String renders Money with full fixed precision (e.g. "123.4500").
func (m Money) String() string { return formatFixed(int64(m), moneyScale) }

// String renders Quantity with full fixed precision (e.g. "1.500").
func (q Quantity) String() string { return formatFixed(int64(q), quantityScale) }

// Add returns m+o, overflow-checked.
func (m Money) Add(o Money) (Money, error) {
	s, ok := addI64(int64(m), int64(o))
	if !ok {
		return 0, ErrOverflow
	}
	return Money(s), nil
}

// Sub returns m-o, overflow-checked.
func (m Money) Sub(o Money) (Money, error) {
	s, ok := subI64(int64(m), int64(o))
	if !ok {
		return 0, ErrOverflow
	}
	return Money(s), nil
}

// IsNegative reports whether the amount is below zero.
func (m Money) IsNegative() bool { return m < 0 }

// IsZero reports whether the amount is exactly zero.
func (m Money) IsZero() bool { return m == 0 }

// MulQuantity returns the notional value of q shares at per-share price m, with
// a 256-bit intermediate so price*qty never overflows before the scale divide.
func (m Money) MulQuantity(q Quantity) (Money, error) {
	v, ok := mulDivRound(int64(m), int64(q), quantityScale)
	if !ok {
		return 0, ErrOverflow
	}
	return Money(v), nil
}

// DivByQuantity returns the per-share price implied by this notional over q
// shares (the inverse of MulQuantity), used for VWAP / average-cost math.
func (m Money) DivByQuantity(q Quantity) (Money, error) {
	if q == 0 {
		return 0, ErrOverflow
	}
	v, ok := mulDivRound(int64(m), quantityScale, int64(q))
	if !ok {
		return 0, ErrOverflow
	}
	return Money(v), nil
}

// Add returns q+o, overflow-checked.
func (q Quantity) Add(o Quantity) (Quantity, error) {
	s, ok := addI64(int64(q), int64(o))
	if !ok {
		return 0, ErrOverflow
	}
	return Quantity(s), nil
}

// Sub returns q-o, overflow-checked.
func (q Quantity) Sub(o Quantity) (Quantity, error) {
	s, ok := subI64(int64(q), int64(o))
	if !ok {
		return 0, ErrOverflow
	}
	return Quantity(s), nil
}

// IsPositive reports whether the quantity is strictly greater than zero.
func (q Quantity) IsPositive() bool { return q > 0 }

// IsZero reports whether the quantity is exactly zero.
func (q Quantity) IsZero() bool { return q == 0 }

// parseFixed parses a signed decimal string into scaled integer minor units.
func parseFixed(s string, scale int64) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty literal")
	}
	neg := false
	switch s[0] {
	case '-':
		neg = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	intPart, fracPart := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
	}
	if intPart == "" && fracPart == "" {
		return 0, errors.New("no digits")
	}
	digits := scaleDigits(scale)
	if len(fracPart) > digits {
		return 0, errors.New("too many fractional digits")
	}
	for len(fracPart) < digits {
		fracPart += "0"
	}
	var whole int64
	if intPart != "" {
		w, err := strconv.ParseInt(intPart, 10, 64)
		if err != nil {
			return 0, err
		}
		whole = w
	}
	var frac int64
	if fracPart != "" {
		f, err := strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return 0, err
		}
		frac = f
	}
	prod, ok := mulI64(whole, scale)
	if !ok {
		return 0, ErrOverflow
	}
	sum, ok := addI64(prod, frac)
	if !ok {
		return 0, ErrOverflow
	}
	if neg {
		sum = -sum
	}
	return sum, nil
}

// formatFixed renders scaled integer minor units as a decimal string.
func formatFixed(v, scale int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	whole := v / scale
	frac := v % scale
	out := strconv.FormatInt(whole, 10)
	if digits := scaleDigits(scale); digits > 0 {
		fs := strconv.FormatInt(frac, 10)
		for len(fs) < digits {
			fs = "0" + fs
		}
		out += "." + fs
	}
	if neg {
		out = "-" + out
	}
	return out
}

// scaleDigits returns the number of fractional digits implied by scale.
func scaleDigits(scale int64) int {
	digits := 0
	for t := scale; t > 1; t /= 10 {
		digits++
	}
	return digits
}

// addI64 adds two int64 values, reporting overflow.
func addI64(a, b int64) (int64, bool) {
	s := a + b
	if (a > 0 && b > 0 && s < 0) || (a < 0 && b < 0 && s > 0) {
		return 0, false
	}
	return s, true
}

// subI64 subtracts two int64 values, reporting overflow.
func subI64(a, b int64) (int64, bool) {
	d := a - b
	if (a >= 0 && b < 0 && d < 0) || (a < 0 && b > 0 && d >= 0) {
		return 0, false
	}
	return d, true
}

// mulI64 multiplies two int64 values, reporting overflow.
func mulI64(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	p := a * b
	if p/b != a {
		return 0, false
	}
	return p, true
}

// mulDivRound computes round(a*b/d) using a big.Int intermediate (overflow-safe)
// with half-away-from-zero rounding, returning false if the result exceeds int64.
func mulDivRound(a, b, d int64) (int64, bool) {
	if d == 0 {
		return 0, false
	}
	x := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	dd := big.NewInt(d)
	q := new(big.Int)
	r := new(big.Int)
	q.QuoRem(x, dd, r)
	twoAbsR := new(big.Int).Abs(r)
	twoAbsR.Lsh(twoAbsR, 1)
	if twoAbsR.Cmp(new(big.Int).Abs(dd)) >= 0 {
		if x.Sign()*dd.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	if !q.IsInt64() {
		return 0, false
	}
	return q.Int64(), true
}
