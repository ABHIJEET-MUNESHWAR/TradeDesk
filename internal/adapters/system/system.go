// Package system provides adapters for host services: a wall-clock and a
// UUID-based id generator. They are trivial but isolating them behind ports
// keeps the core deterministic and testable.
package system

import (
	"time"

	"github.com/google/uuid"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/ports"
)

// Clock is the production wall-clock adapter.
type Clock struct{}

// Now returns the current UTC time.
func (Clock) Now() time.Time { return time.Now().UTC() }

// IDGenerator produces UUIDv4 order identifiers.
type IDGenerator struct{}

// NewID returns a fresh unique id.
func (IDGenerator) NewID() string { return "ord-" + uuid.NewString() }

var (
	_ ports.Clock       = Clock{}
	_ ports.IDGenerator = IDGenerator{}
)
