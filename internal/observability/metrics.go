package observability

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds the Prometheus instruments for the service. Grouping them keeps
// wiring in one place and lets handlers/services increment counters cheaply.
type Metrics struct {
	OrdersPlaced    *prometheus.CounterVec
	OrdersFilled    prometheus.Counter
	OrdersCanceled  prometheus.Counter
	RiskChecks      *prometheus.CounterVec
	RouteFailures   prometheus.Counter
	GraphQLRequests *prometheus.CounterVec
	PlaceLatency    prometheus.Histogram
}

// NewMetrics registers and returns the service metrics on reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		OrdersPlaced: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tradedesk_orders_placed_total",
			Help: "Orders placed, labelled by resulting status.",
		}, []string{"status"}),
		OrdersFilled: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tradedesk_orders_filled_total",
			Help: "Orders that reached the FILLED state.",
		}),
		OrdersCanceled: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tradedesk_orders_canceled_total",
			Help: "Orders that were cancelled.",
		}),
		RiskChecks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tradedesk_risk_checks_total",
			Help: "Pre-trade risk checks, labelled by outcome.",
		}, []string{"outcome"}),
		RouteFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tradedesk_route_failures_total",
			Help: "Venue routing failures after retries.",
		}),
		GraphQLRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tradedesk_graphql_requests_total",
			Help: "GraphQL operations, labelled by operation and result.",
		}, []string{"operation", "result"}),
		PlaceLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "tradedesk_place_order_seconds",
			Help:    "PlaceOrder end-to-end latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
	}
	reg.MustRegister(
		m.OrdersPlaced, m.OrdersFilled, m.OrdersCanceled,
		m.RiskChecks, m.RouteFailures, m.GraphQLRequests, m.PlaceLatency,
	)
	return m
}
