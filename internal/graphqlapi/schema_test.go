package graphqlapi

import (
	"context"
	"testing"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/adapters/ai"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/adapters/memory"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/adapters/system"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/app"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/observability"
)

func newTestSchema(t *testing.T) graphql.Schema {
	t.Helper()
	reg := prometheus.NewRegistry()
	metrics := observability.NewMetrics(reg)
	repo := memory.NewOrderRepository(8)
	bus := memory.NewEventBus()
	rm := memory.NewPositionReadModel()
	bus.SubscribeAll(rm.Project)
	svc := app.NewService(app.Deps{
		Repo:      repo,
		Publisher: bus,
		Risk:      ai.NewHeuristicAnalyzer(ai.DefaultThresholds()),
		Venue:     memory.NewSimulatedVenue(),
		Positions: rm,
		Clock:     system.Clock{},
		IDs:       system.IDGenerator{},
		Logger:    observability.NewLogger("error"),
		Metrics:   metrics,
	}, app.Config{
		RiskTimeout: time.Second, RouteTimeout: time.Second,
		RetryAttempts: 2, RetryBase: time.Millisecond, RetryMax: 5 * time.Millisecond,
		RateLimitRPS: 1000, RateLimitBurst: 1000,
		BreakerFailures: 5, BreakerCooldown: time.Second,
	})
	schema, err := NewSchema(svc, metrics)
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}
	return schema
}

func exec(t *testing.T, schema graphql.Schema, query string) *graphql.Result {
	t.Helper()
	res := graphql.Do(graphql.Params{Schema: schema, RequestString: query, Context: context.Background()})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %v", res.Errors)
	}
	return res
}

func TestGraphQLPlaceQueryAndFill(t *testing.T) {
	schema := newTestSchema(t)

	res := exec(t, schema, `mutation {
		placeOrder(accountId:"acct-1", symbol:"AAPL", side:"BUY", orderType:"LIMIT", quantity:"10", limitPrice:"150.00") {
			orderId status idempotent
		}
	}`)
	place := res.Data.(map[string]interface{})["placeOrder"].(map[string]interface{})
	if place["status"] != "ACCEPTED" {
		t.Fatalf("status=%v", place["status"])
	}
	orderID := place["orderId"].(string)

	res = exec(t, schema, `query { order(id:"`+orderID+`") { id status quantity } }`)
	order := res.Data.(map[string]interface{})["order"].(map[string]interface{})
	if order["status"] != "ACCEPTED" || order["quantity"] != "10.000" {
		t.Fatalf("order=%v", order)
	}

	res = exec(t, schema, `mutation { applyFill(orderId:"`+orderID+`", quantity:"10", price:"150.00") { status avgFillPrice } }`)
	fill := res.Data.(map[string]interface{})["applyFill"].(map[string]interface{})
	if fill["status"] != "FILLED" || fill["avgFillPrice"] != "150.0000" {
		t.Fatalf("fill=%v", fill)
	}
}

func TestGraphQLHealthAndUnknownOrder(t *testing.T) {
	schema := newTestSchema(t)
	res := exec(t, schema, `query { health { status breaker } }`)
	health := res.Data.(map[string]interface{})["health"].(map[string]interface{})
	if health["status"] != "ok" {
		t.Fatalf("health=%v", health)
	}
	// Unknown order resolves to null (not an error).
	res = exec(t, schema, `query { order(id:"does-not-exist") { id } }`)
	if res.Data.(map[string]interface{})["order"] != nil {
		t.Fatal("expected null for unknown order")
	}
}
