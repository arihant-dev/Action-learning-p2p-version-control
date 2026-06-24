// Package versioning provides logical clock implementations (Lamport
// and vector clocks) and a Last-Write-Wins conflict resolver for the
// P2P file synchronization system.
package versioning

import (
	"encoding/json"
	"sync"
)

// LamportClock is a thread-safe monotonically increasing logical clock.
// Every local event increments the counter. When a message arrives from
// a remote peer carrying a remote timestamp, the clock is updated to
// max(local, remote) + 1 so that causality is preserved.
type LamportClock struct {
	mu      sync.Mutex
	counter uint64
}

// NewLamportClock returns a clock initialised to zero.
func NewLamportClock() *LamportClock {
	return &LamportClock{}
}

// NewLamportClockAt returns a clock initialised to the given value.
// This is useful when restoring state from the SQLite database.
func NewLamportClockAt(value uint64) *LamportClock {
	return &LamportClock{counter: value}
}

// Tick increments the clock for a local event and returns the new value.
func (c *LamportClock) Tick() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter++
	return c.counter
}

// Witness updates the clock after receiving a remote timestamp.
// The clock is set to max(local, remote) + 1.
func (c *LamportClock) Witness(remote uint64) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if remote > c.counter {
		c.counter = remote
	}
	c.counter++
	return c.counter
}

// Value returns the current clock value without incrementing.
func (c *LamportClock) Value() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counter
}

// MarshalJSON implements json.Marshaler.
func (c *LamportClock) MarshalJSON() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return json.Marshal(c.counter)
}

// UnmarshalJSON implements json.Unmarshaler.
func (c *LamportClock) UnmarshalJSON(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return json.Unmarshal(data, &c.counter)
}
