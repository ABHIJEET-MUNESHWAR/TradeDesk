package graphqlapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestHTTPServerOperationalRoutes(t *testing.T) {
	schema := newTestSchema(t)
	reg := prometheus.NewRegistry()
	srv := NewHTTPServer(":0", schema, reg, time.Second)

	cases := []struct {
		path string
		want int
	}{
		{"/healthz", http.StatusOK},
		{"/readyz", http.StatusOK},
		{"/metrics", http.StatusOK},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: got %d want %d", tc.path, rec.Code, tc.want)
		}
	}
}

func TestGraphQLEndpointServes(t *testing.T) {
	schema := newTestSchema(t)
	srv := NewHTTPServer(":0", schema, prometheus.NewRegistry(), time.Second)
	body := `{"query":"query { health { status } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("graphql POST got %d", rec.Code)
	}
}
