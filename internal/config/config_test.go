package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	os.Clearenv()
	c := Load()
	if c.HTTPAddr != ":8080" {
		t.Fatalf("addr=%s", c.HTTPAddr)
	}
	if c.RepoShards != 16 {
		t.Fatalf("shards=%d", c.RepoShards)
	}
	if c.RiskTimeout != 2*time.Second {
		t.Fatalf("riskTimeout=%s", c.RiskTimeout)
	}
	if c.RateLimitRPS != 500 {
		t.Fatalf("rps=%v", c.RateLimitRPS)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	os.Clearenv()
	t.Setenv("TRADEDESK_HTTP_ADDR", ":9999")
	t.Setenv("TRADEDESK_REPO_SHARDS", "32")
	t.Setenv("TRADEDESK_RISK_TIMEOUT", "5s")
	t.Setenv("TRADEDESK_RATE_LIMIT_RPS", "250.5")
	c := Load()
	if c.HTTPAddr != ":9999" || c.RepoShards != 32 || c.RiskTimeout != 5*time.Second || c.RateLimitRPS != 250.5 {
		t.Fatalf("override failed: %+v", c)
	}
}

func TestEmptyEnvTreatedAsUnset(t *testing.T) {
	os.Clearenv()
	t.Setenv("TRADEDESK_HTTP_ADDR", "")
	if Load().HTTPAddr != ":8080" {
		t.Fatal("empty env var should fall back to default")
	}
}

func TestInvalidEnvFallsBack(t *testing.T) {
	os.Clearenv()
	t.Setenv("TRADEDESK_REPO_SHARDS", "not-a-number")
	t.Setenv("TRADEDESK_RISK_TIMEOUT", "not-a-duration")
	c := Load()
	if c.RepoShards != 16 || c.RiskTimeout != 2*time.Second {
		t.Fatalf("invalid values should fall back to defaults: %+v", c)
	}
}
