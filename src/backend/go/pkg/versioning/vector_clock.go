package versioning

import (
	"encoding/json"
	"sync"
)

// Ordering describes the causal relationship between two vector clocks.
type Ordering int

const (
	// Equal means the two clocks are identical.
	Equal Ordering = iota
	// Before means the left clock happened-before the right clock.
	Before
	// After means the left clock happened-after the right clock.
	After
	// Concurrent means neither clock happened-before the other;
	// the events are causally independent.
	Concurrent
)

// String returns a human-readable label for the ordering.
func (o Ordering) String() string {
	switch o {
	case Equal:
		return "Equal"
	case Before:
		return "Before"
	case After:
		return "After"
	case Concurrent:
		return "Concurrent"
	default:
		return "Unknown"
	}
}

// VectorClock tracks causal dependencies across a set of peers. Each
// peer maintains its own entry (counter) in the map. The type is
// compatible with the map[string]uint64 used in pkg/protocol.
type VectorClock struct {
	mu     sync.RWMutex
	clocks map[string]uint64
}

// NewVectorClock returns an empty vector clock.
func NewVectorClock() *VectorClock {
	return &VectorClock{clocks: make(map[string]uint64)}
}

// NewVectorClockFrom creates a vector clock from an existing map.
// The map is copied so the caller retains ownership of the original.
func NewVectorClockFrom(m map[string]uint64) *VectorClock {
	vc := &VectorClock{clocks: make(map[string]uint64, len(m))}
	for k, v := range m {
		vc.clocks[k] = v
	}
	return vc
}

// Tick increments the counter for the given peer and returns the new
// value. This should be called on every local event.
func (vc *VectorClock) Tick(peerID string) uint64 {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.clocks[peerID]++
	return vc.clocks[peerID]
}

// Merge updates each component to the maximum of the local and remote
// values. Call this when a message is received from a remote peer.
func (vc *VectorClock) Merge(remote map[string]uint64) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	for k, v := range remote {
		if v > vc.clocks[k] {
			vc.clocks[k] = v
		}
	}
}

// Get returns the counter for a specific peer.
func (vc *VectorClock) Get(peerID string) uint64 {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.clocks[peerID]
}

// AsMap returns a copy of the internal clock map, suitable for
// embedding in protocol messages.
func (vc *VectorClock) AsMap() map[string]uint64 {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	m := make(map[string]uint64, len(vc.clocks))
	for k, v := range vc.clocks {
		m[k] = v
	}
	return m
}

// Compare determines the causal ordering between this clock (left) and
// another clock (right).
//
//   - Equal:      all components are identical
//   - Before:     all components ≤ right, and at least one is strictly <
//   - After:      all components ≥ right, and at least one is strictly >
//   - Concurrent: some components are < and some are >
func (vc *VectorClock) Compare(other *VectorClock) Ordering {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	// Collect the union of all keys.
	keys := make(map[string]struct{})
	for k := range vc.clocks {
		keys[k] = struct{}{}
	}
	for k := range other.clocks {
		keys[k] = struct{}{}
	}

	hasLess := false
	hasGreater := false

	for k := range keys {
		l := vc.clocks[k]
		r := other.clocks[k]
		if l < r {
			hasLess = true
		}
		if l > r {
			hasGreater = true
		}
		if hasLess && hasGreater {
			return Concurrent
		}
	}

	switch {
	case !hasLess && !hasGreater:
		return Equal
	case hasLess && !hasGreater:
		return Before
	default:
		return After
	}
}

// MarshalJSON implements json.Marshaler — serializes to a JSON object.
func (vc *VectorClock) MarshalJSON() ([]byte, error) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return json.Marshal(vc.clocks)
}

// UnmarshalJSON implements json.Unmarshaler.
func (vc *VectorClock) UnmarshalJSON(data []byte) error {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	if vc.clocks == nil {
		vc.clocks = make(map[string]uint64)
	}
	return json.Unmarshal(data, &vc.clocks)
}
