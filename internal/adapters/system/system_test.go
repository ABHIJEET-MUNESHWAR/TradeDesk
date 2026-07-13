package system

import (
	"strings"
	"testing"
	"time"
)

func TestClockNow(t *testing.T) {
	before := time.Now().Add(-time.Second)
	got := Clock{}.Now()
	if got.Before(before) {
		t.Fatalf("clock returned stale time %v", got)
	}
	if got.Location() != time.UTC {
		t.Fatalf("expected UTC, got %v", got.Location())
	}
}

func TestIDGeneratorUnique(t *testing.T) {
	g := IDGenerator{}
	seen := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		id := g.NewID()
		if !strings.HasPrefix(id, "ord-") {
			t.Fatalf("id missing prefix: %s", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id: %s", id)
		}
		seen[id] = struct{}{}
	}
}
