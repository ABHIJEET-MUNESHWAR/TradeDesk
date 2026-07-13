package graphqlapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewHTTPServer builds the HTTP server exposing GraphQL at /graphql (with the
// GraphiQL playground), liveness at /healthz, readiness at /readyz, and
// Prometheus metrics at /metrics.
func NewHTTPServer(addr string, schema graphql.Schema, reg *prometheus.Registry, timeout time.Duration) *http.Server {
	gqlHandler := handler.New(&handler.Config{
		Schema:     &schema,
		Pretty:     true,
		GraphiQL:   true,
		Playground: false,
	})

	mux := http.NewServeMux()
	mux.Handle("/graphql", gqlHandler)
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	return &http.Server{
		Addr:              addr,
		Handler:           recoverMiddleware(mux),
		ReadHeaderTimeout: timeout,
		ReadTimeout:       timeout,
		WriteTimeout:      2 * timeout,
		IdleTimeout:       4 * timeout,
	}
}

// recoverMiddleware converts a panic in any handler into a 500 so a single bad
// request can never take down the process (fault isolation at the edge).
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ShutdownContext returns a context for graceful shutdown of the server.
func ShutdownContext(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
