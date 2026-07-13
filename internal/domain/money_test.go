package domain

import "testing"

func TestParseAndFormatMoney(t *testing.T) {
	cases := []struct{ in, want string }{
		{"123.45", "123.4500"},
		{"0", "0.0000"},
		{"-5.5", "-5.5000"},
		{"1000000", "1000000.0000"},
		{"0.0001", "0.0001"},
	}
	for _, c := range cases {
		m, err := ParseMoney(c.in)
		if err != nil {
			t.Fatalf("ParseMoney(%q): %v", c.in, err)
		}
		if got := m.String(); got != c.want {
			t.Errorf("ParseMoney(%q).String()=%q want %q", c.in, got, c.want)
		}
	}
}

func TestParseMoneyInvalid(t *testing.T) {
	for _, in := range []string{"", "abc", "1.23456", "1.2.3", "  "} {
		if _, err := ParseMoney(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

func TestParseQuantity(t *testing.T) {
	q, err := ParseQuantity("2.5")
	if err != nil {
		t.Fatal(err)
	}
	if q.String() != "2.500" {
		t.Fatalf("got %q", q.String())
	}
	if !q.IsPositive() || q.IsZero() {
		t.Fatal("2.5 should be positive and non-zero")
	}
}

func TestMoneyArithmetic(t *testing.T) {
	a, _ := ParseMoney("10.00")
	b, _ := ParseMoney("2.50")
	sum, err := a.Add(b)
	if err != nil || sum.String() != "12.5000" {
		t.Fatalf("add=%v %v", sum, err)
	}
	diff, err := a.Sub(b)
	if err != nil || diff.String() != "7.5000" {
		t.Fatalf("sub=%v %v", diff, err)
	}
}

func TestMulQuantity(t *testing.T) {
	px, _ := ParseMoney("100.00")
	qty, _ := ParseQuantity("2.5")
	notional, err := px.MulQuantity(qty)
	if err != nil {
		t.Fatal(err)
	}
	if notional.String() != "250.0000" {
		t.Fatalf("notional=%s", notional.String())
	}
}

func TestDivByQuantity(t *testing.T) {
	notional, _ := ParseMoney("250.00")
	qty, _ := ParseQuantity("2.5")
	px, err := notional.DivByQuantity(qty)
	if err != nil {
		t.Fatal(err)
	}
	if px.String() != "100.0000" {
		t.Fatalf("px=%s", px.String())
	}
	if _, err := notional.DivByQuantity(0); err == nil {
		t.Fatal("expected divide-by-zero error")
	}
}

func TestMoneyOverflow(t *testing.T) {
	maxVal := Money(9223372036854775807)
	if _, err := maxVal.Add(Money(1)); err == nil {
		t.Fatal("expected add overflow")
	}
	minVal := Money(-9223372036854775808)
	if _, err := minVal.Sub(Money(1)); err == nil {
		t.Fatal("expected sub overflow")
	}
}

func TestMoneyPredicates(t *testing.T) {
	neg, _ := ParseMoney("-1")
	if !neg.IsNegative() {
		t.Fatal("-1 should be negative")
	}
	if !Money(0).IsZero() {
		t.Fatal("0 should be zero")
	}
}
