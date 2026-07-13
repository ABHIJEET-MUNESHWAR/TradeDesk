package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewLoggerAllLevels(t *testing.T) {
	for _, lvl := range []string{"debug", "info", "warn", "error", "unknown"} {
		if NewLogger(lvl) == nil {
			t.Fatalf("nil logger for level %q", lvl)
		}
	}
}

func TestNewMetricsRegistersAndIncrements(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	if m == nil {
		t.Fatal("nil metrics")
	}
	m.OrdersFilled.Inc()
	m.OrdersCanceled.Inc()
	m.OrdersPlaced.WithLabelValues("ACCEPTED").Inc()
	m.RiskChecks.WithLabelValues("approved").Inc()
	m.RouteFailures.Inc()
	m.GraphQLRequests.WithLabelValues("placeOrder", "ok").Inc()
	m.PlaceLatency.Observe(0.01)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(mfs) == 0 {
		t.Fatal("expected registered metric families")
	}
}
