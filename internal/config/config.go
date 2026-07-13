// Package config loads twelve-factor configuration from the environment with
// safe production defaults, so the binary is configured entirely via env vars.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config is the fully-resolved service configuration.
type Config struct {
	HTTPAddr        string
	LogLevel        string
	HTTPTimeout     time.Duration
	RiskTimeout     time.Duration
	RouteTimeout    time.Duration
	ShutdownTimeout time.Duration
	RateLimitRPS    float64
	RateLimitBurst  int
	BreakerFailures int
	BreakerCooldown time.Duration
	RetryAttempts   int
	RepoShards      int
	AIProvider      string // "heuristic" | "llm"
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		HTTPAddr:        getEnv("TRADEDESK_HTTP_ADDR", ":8080"),
		LogLevel:        getEnv("TRADEDESK_LOG_LEVEL", "info"),
		HTTPTimeout:     getEnvDuration("TRADEDESK_HTTP_TIMEOUT", 15*time.Second),
		RiskTimeout:     getEnvDuration("TRADEDESK_RISK_TIMEOUT", 2*time.Second),
		RouteTimeout:    getEnvDuration("TRADEDESK_ROUTE_TIMEOUT", 3*time.Second),
		ShutdownTimeout: getEnvDuration("TRADEDESK_SHUTDOWN_TIMEOUT", 10*time.Second),
		RateLimitRPS:    getEnvFloat("TRADEDESK_RATE_LIMIT_RPS", 500),
		RateLimitBurst:  getEnvInt("TRADEDESK_RATE_LIMIT_BURST", 200),
		BreakerFailures: getEnvInt("TRADEDESK_BREAKER_FAILURES", 5),
		BreakerCooldown: getEnvDuration("TRADEDESK_BREAKER_COOLDOWN", 5*time.Second),
		RetryAttempts:   getEnvInt("TRADEDESK_RETRY_ATTEMPTS", 3),
		RepoShards:      getEnvInt("TRADEDESK_REPO_SHARDS", 16),
		AIProvider:      getEnv("TRADEDESK_AI_PROVIDER", "heuristic"),
	}
}

// getEnv returns the env value for key, or def if unset/empty (twelve-factor:
// an empty string is treated as unset).
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
