package graphqlapi

import (
	"errors"

	"github.com/graphql-go/graphql"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/app"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/observability"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/ports"
)

// Resolver holds the dependencies the GraphQL field resolvers need.
type Resolver struct {
	svc     *app.Service
	metrics *observability.Metrics
}

// NewSchema builds the GraphQL schema. The read/write surface (order, orders,
// positions, health + placeOrder, cancelOrder, applyFill) is >5 operations, so
// GraphQL is used per the project guidelines. Object field resolution relies on
// graphql-go's reflection default resolver against the app view structs.
func NewSchema(svc *app.Service, metrics *observability.Metrics) (graphql.Schema, error) {
	r := &Resolver{svc: svc, metrics: metrics}

	orderType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Order",
		Fields: graphql.Fields{
			"id":           &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"accountId":    &graphql.Field{Type: graphql.String},
			"symbol":       &graphql.Field{Type: graphql.String},
			"side":         &graphql.Field{Type: graphql.String},
			"orderType":    &graphql.Field{Type: graphql.String},
			"tif":          &graphql.Field{Type: graphql.String},
			"limitPrice":   &graphql.Field{Type: graphql.String},
			"quantity":     &graphql.Field{Type: graphql.String},
			"filledQty":    &graphql.Field{Type: graphql.String},
			"avgFillPrice": &graphql.Field{Type: graphql.String},
			"status":       &graphql.Field{Type: graphql.String},
			"version":      &graphql.Field{Type: graphql.Int},
			"createdAt":    &graphql.Field{Type: graphql.DateTime},
			"updatedAt":    &graphql.Field{Type: graphql.DateTime},
		},
	})

	positionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Position",
		Fields: graphql.Fields{
			"accountId": &graphql.Field{Type: graphql.String},
			"symbol":    &graphql.Field{Type: graphql.String},
			"netQty":    &graphql.Field{Type: graphql.String},
			"avgPrice":  &graphql.Field{Type: graphql.String},
		},
	})

	placeResultType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PlaceOrderResult",
		Fields: graphql.Fields{
			"orderId":    &graphql.Field{Type: graphql.String},
			"status":     &graphql.Field{Type: graphql.String},
			"idempotent": &graphql.Field{Type: graphql.Boolean},
		},
	})

	healthType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Health",
		Fields: graphql.Fields{
			"status":  &graphql.Field{Type: graphql.String},
			"breaker": &graphql.Field{Type: graphql.String},
		},
	})

	query := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"health": &graphql.Field{
				Type:    healthType,
				Resolve: r.resolveHealth,
			},
			"order": &graphql.Field{
				Type:    orderType,
				Args:    graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}},
				Resolve: r.resolveOrder,
			},
			"orders": &graphql.Field{
				Type:    graphql.NewList(orderType),
				Args:    graphql.FieldConfigArgument{"limit": &graphql.ArgumentConfig{Type: graphql.Int, DefaultValue: 50}},
				Resolve: r.resolveOrders,
			},
			"positions": &graphql.Field{
				Type:    graphql.NewList(positionType),
				Args:    graphql.FieldConfigArgument{"accountId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}},
				Resolve: r.resolvePositions,
			},
		},
	})

	mutation := graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"placeOrder": &graphql.Field{
				Type: placeResultType,
				Args: graphql.FieldConfigArgument{
					"accountId":      &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"symbol":         &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"side":           &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"orderType":      &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"tif":            &graphql.ArgumentConfig{Type: graphql.String, DefaultValue: "DAY"},
					"quantity":       &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"limitPrice":     &graphql.ArgumentConfig{Type: graphql.String, DefaultValue: ""},
					"idempotencyKey": &graphql.ArgumentConfig{Type: graphql.String, DefaultValue: ""},
				},
				Resolve: r.resolvePlaceOrder,
			},
			"cancelOrder": &graphql.Field{
				Type:    orderType,
				Args:    graphql.FieldConfigArgument{"orderId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}},
				Resolve: r.resolveCancelOrder,
			},
			"applyFill": &graphql.Field{
				Type: orderType,
				Args: graphql.FieldConfigArgument{
					"orderId":  &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"quantity": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"price":    &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: r.resolveApplyFill,
			},
		},
	})

	return graphql.NewSchema(graphql.SchemaConfig{Query: query, Mutation: mutation})
}

// track records a GraphQL operation metric around a resolver body.
func (r *Resolver) track(op string, fn func() (interface{}, error)) (interface{}, error) {
	res, err := fn()
	result := "ok"
	if err != nil {
		result = "error"
	}
	r.metrics.GraphQLRequests.WithLabelValues(op, result).Inc()
	return res, err
}

func (r *Resolver) resolveHealth(p graphql.ResolveParams) (interface{}, error) {
	return r.track("health", func() (interface{}, error) {
		return map[string]interface{}{"status": "ok", "breaker": r.svc.BreakerState()}, nil
	})
}

func (r *Resolver) resolveOrder(p graphql.ResolveParams) (interface{}, error) {
	return r.track("order", func() (interface{}, error) {
		id, _ := p.Args["id"].(string)
		view, err := r.svc.GetOrder(p.Context, id)
		if errors.Is(err, ports.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return view, nil
	})
}

func (r *Resolver) resolveOrders(p graphql.ResolveParams) (interface{}, error) {
	return r.track("orders", func() (interface{}, error) {
		limit, _ := p.Args["limit"].(int)
		return r.svc.ListOrders(p.Context, limit)
	})
}

func (r *Resolver) resolvePositions(p graphql.ResolveParams) (interface{}, error) {
	return r.track("positions", func() (interface{}, error) {
		accountID, _ := p.Args["accountId"].(string)
		return r.svc.Positions(p.Context, accountID)
	})
}

func (r *Resolver) resolvePlaceOrder(p graphql.ResolveParams) (interface{}, error) {
	return r.track("placeOrder", func() (interface{}, error) {
		cmd := app.PlaceOrderCommand{
			AccountID:      argString(p, "accountId"),
			Symbol:         argString(p, "symbol"),
			Side:           argString(p, "side"),
			OrderType:      argString(p, "orderType"),
			TIF:            argString(p, "tif"),
			Quantity:       argString(p, "quantity"),
			LimitPrice:     argString(p, "limitPrice"),
			IdempotencyKey: argString(p, "idempotencyKey"),
		}
		return r.svc.PlaceOrder(p.Context, cmd)
	})
}

func (r *Resolver) resolveCancelOrder(p graphql.ResolveParams) (interface{}, error) {
	return r.track("cancelOrder", func() (interface{}, error) {
		id := argString(p, "orderId")
		if err := r.svc.CancelOrder(p.Context, app.CancelOrderCommand{OrderID: id}); err != nil {
			return nil, err
		}
		return r.svc.GetOrder(p.Context, id)
	})
}

func (r *Resolver) resolveApplyFill(p graphql.ResolveParams) (interface{}, error) {
	return r.track("applyFill", func() (interface{}, error) {
		id := argString(p, "orderId")
		cmd := app.ApplyFillCommand{OrderID: id, Quantity: argString(p, "quantity"), Price: argString(p, "price")}
		if err := r.svc.ApplyFill(p.Context, cmd); err != nil {
			return nil, err
		}
		return r.svc.GetOrder(p.Context, id)
	})
}

// argString reads a string argument, tolerating absence.
func argString(p graphql.ResolveParams, name string) string {
	v, _ := p.Args[name].(string)
	return v
}
