// Command tradedesk is the composition root: it wires adapters to the
// application core and serves the GraphQL API with graceful shutdown.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/adapters/ai"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/adapters/memory"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/adapters/system"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/app"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/config"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/graphqlapi"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/observability"
)

func main() {
	healthCheck := flag.Bool("health", false, "run an internal health probe and exit (used by the container HEALTHCHECK)")
	flag.Parse()

	cfg := config.Load()
	if *healthCheck {
		os.Exit(runHealthProbe(cfg.HTTPAddr))
	}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

// run builds the service and blocks until a shutdown signal is received.
func run(cfg config.Config) error {
	logger := observability.NewLogger(cfg.LogLevel)
	reg := prometheus.NewRegistry()
	metrics := observability.NewMetrics(reg)

	repo := memory.NewOrderRepository(cfg.RepoShards)
	bus := memory.NewEventBus()
	readModel := memory.NewPositionReadModel()
	bus.SubscribeAll(readModel.Project)
	venue := memory.NewSimulatedVenue()
	risk := ai.NewHeuristicAnalyzer(ai.DefaultThresholds())

	svc := app.NewService(app.Deps{
		Repo:      repo,
		Publisher: bus,
		Risk:      risk,
		Venue:     venue,
		Positions: readModel,
		Clock:     system.Clock{},
		IDs:       system.IDGenerator{},
		Logger:    logger,
		Metrics:   metrics,
	}, app.Config{
		RiskTimeout:     cfg.RiskTimeout,
		RouteTimeout:    cfg.RouteTimeout,
		RetryAttempts:   cfg.RetryAttempts,
		RetryBase:       20 * time.Millisecond,
		RetryMax:        500 * time.Millisecond,
		RateLimitRPS:    cfg.RateLimitRPS,
		RateLimitBurst:  cfg.RateLimitBurst,
		BreakerFailures: cfg.BreakerFailures,
		BreakerCooldown: cfg.BreakerCooldown,
	})

	schema, err := graphqlapi.NewSchema(svc, metrics)
	if err != nil {
		return fmt.Errorf("build schema: %w", err)
	}
	srv := graphqlapi.NewHTTPServer(cfg.HTTPAddr, schema, reg, cfg.HTTPTimeout)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("tradedesk listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := graphqlapi.ShutdownContext(cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	bus.Close()
	logger.Info("stopped cleanly")
	return nil
}

// runHealthProbe performs a self HTTP GET against /healthz and returns a process
// exit code. Distroless images have no shell, so the container HEALTHCHECK runs
// the binary itself with --health.
func runHealthProbe(addr string) int {
	host := addr
	if len(host) > 0 && host[0] == ':' {
		host = "127.0.0.1" + host
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + host + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "health probe failed:", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
