package domain

import (
	"testing"
	"time"
)

// BenchmarkNewOrder measures order construction + validation (O(1)).
func BenchmarkNewOrder(b *testing.B) {
	sym, _ := ParseSymbol("AAPL")
	limit, _ := ParseMoney("150.00")
	qty, _ := ParseQuantity("10")
	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o, err := NewOrder("ord-1", "acct-1", sym, SideBuy, OrderTypeLimit, TIFDay, limit, qty, now)
		if err != nil {
			b.Fatal(err)
		}
		_ = o
	}
}

// BenchmarkFillVWAP measures the fixed-point VWAP fill math (O(1) per fill).
func BenchmarkFillVWAP(b *testing.B) {
	sym, _ := ParseSymbol("AAPL")
	limit, _ := ParseMoney("150.00")
	qty, _ := ParseQuantity("1000000")
	one, _ := ParseQuantity("1")
	px, _ := ParseMoney("150.00")
	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o, _ := NewOrder("ord", "acct", sym, SideBuy, OrderTypeLimit, TIFDay, limit, qty, now)
		_ = o.Accept(now)
		_ = o.Fill(one, px, now)
	}
}

// BenchmarkMulQuantity measures the overflow-safe notional multiply.
func BenchmarkMulQuantity(b *testing.B) {
	px, _ := ParseMoney("12345.6789")
	qty, _ := ParseQuantity("987.654")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := px.MulQuantity(qty); err != nil {
			b.Fatal(err)
		}
	}
}
