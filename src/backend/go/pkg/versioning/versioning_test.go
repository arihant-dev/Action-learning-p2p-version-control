package versioning

import (
	"encoding/json"
	"sync"
	"testing"
)

// ===========================================================================
// Lamport clock tests
// ===========================================================================

func TestLamportTick(t *testing.T) {
	c := NewLamportClock()
	if v := c.Tick(); v != 1 {
		t.Errorf("first tick = %d, want 1", v)
	}
	if v := c.Tick(); v != 2 {
		t.Errorf("second tick = %d, want 2", v)
	}
}

func TestLamportWitness(t *testing.T) {
	c := NewLamportClock()
	c.Tick() // counter = 1

	// Witness a remote value larger than local → jump.
	v := c.Witness(10)
	if v != 11 {
		t.Errorf("witness(10) = %d, want 11", v)
	}

	// Witness a remote value smaller than local → local + 1.
	v = c.Witness(5)
	if v != 12 {
		t.Errorf("witness(5) = %d, want 12", v)
	}
}

func TestLamportValue(t *testing.T) {
	c := NewLamportClockAt(42)
	if v := c.Value(); v != 42 {
		t.Errorf("value = %d, want 42", v)
	}
}

func TestLamportJSON(t *testing.T) {
	c := NewLamportClockAt(7)
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	c2 := NewLamportClock()
	if err := json.Unmarshal(data, c2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c2.Value() != 7 {
		t.Errorf("roundtrip value = %d, want 7", c2.Value())
	}
}

func TestLamportConcurrentAccess(t *testing.T) {
	c := NewLamportClock()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Tick()
		}()
	}
	wg.Wait()
	if v := c.Value(); v != 100 {
		t.Errorf("after 100 concurrent ticks, value = %d, want 100", v)
	}
}

// ===========================================================================
// Vector clock tests
// ===========================================================================

func TestVectorClockTick(t *testing.T) {
	vc := NewVectorClock()
	vc.Tick("A")
	vc.Tick("A")
	vc.Tick("B")

	if v := vc.Get("A"); v != 2 {
		t.Errorf("A = %d, want 2", v)
	}
	if v := vc.Get("B"); v != 1 {
		t.Errorf("B = %d, want 1", v)
	}
	if v := vc.Get("C"); v != 0 {
		t.Errorf("C = %d, want 0 (absent)", v)
	}
}

func TestVectorClockMerge(t *testing.T) {
	vc := NewVectorClockFrom(map[string]uint64{"A": 3, "B": 1})
	vc.Merge(map[string]uint64{"A": 1, "B": 5, "C": 2})

	expected := map[string]uint64{"A": 3, "B": 5, "C": 2}
	got := vc.AsMap()
	for k, want := range expected {
		if got[k] != want {
			t.Errorf("%s = %d, want %d", k, got[k], want)
		}
	}
}

func TestVectorClockCompareEqual(t *testing.T) {
	a := NewVectorClockFrom(map[string]uint64{"A": 2, "B": 3})
	b := NewVectorClockFrom(map[string]uint64{"A": 2, "B": 3})
	if o := a.Compare(b); o != Equal {
		t.Errorf("compare = %v, want Equal", o)
	}
}

func TestVectorClockCompareBefore(t *testing.T) {
	a := NewVectorClockFrom(map[string]uint64{"A": 1, "B": 2})
	b := NewVectorClockFrom(map[string]uint64{"A": 2, "B": 3})
	if o := a.Compare(b); o != Before {
		t.Errorf("compare = %v, want Before", o)
	}
}

func TestVectorClockCompareAfter(t *testing.T) {
	a := NewVectorClockFrom(map[string]uint64{"A": 5, "B": 3})
	b := NewVectorClockFrom(map[string]uint64{"A": 2, "B": 3})
	if o := a.Compare(b); o != After {
		t.Errorf("compare = %v, want After", o)
	}
}

func TestVectorClockCompareConcurrent(t *testing.T) {
	a := NewVectorClockFrom(map[string]uint64{"A": 3, "B": 1})
	b := NewVectorClockFrom(map[string]uint64{"A": 1, "B": 3})
	if o := a.Compare(b); o != Concurrent {
		t.Errorf("compare = %v, want Concurrent", o)
	}
}

func TestVectorClockCompareMissingKeys(t *testing.T) {
	// a has a key that b doesn't — a > b on that component.
	a := NewVectorClockFrom(map[string]uint64{"A": 1, "B": 1})
	b := NewVectorClockFrom(map[string]uint64{"A": 1})
	if o := a.Compare(b); o != After {
		t.Errorf("compare = %v, want After (A has B=1, B has B=0)", o)
	}
}

func TestVectorClockJSON(t *testing.T) {
	vc := NewVectorClockFrom(map[string]uint64{"peer-A": 3, "peer-B": 7})
	data, err := json.Marshal(vc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	vc2 := NewVectorClock()
	if err := json.Unmarshal(data, vc2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if vc2.Get("peer-A") != 3 || vc2.Get("peer-B") != 7 {
		t.Errorf("roundtrip mismatch: %v", vc2.AsMap())
	}
}

// ===========================================================================
// Conflict detector tests
// ===========================================================================

func TestResolveIdenticalFiles(t *testing.T) {
	cd := NewConflictDetector()
	r := cd.Resolve(
		FileVersion{Hash: "abc123", LamportVersion: 5, Timestamp: 1000, PeerID: "A"},
		FileVersion{Hash: "abc123", LamportVersion: 8, Timestamp: 2000, PeerID: "B"},
	)
	if r.Action != Skip {
		t.Errorf("action = %v, want Skip", r.Action)
	}
	if r.IsConflict {
		t.Error("identical files should not be marked as conflict")
	}
}

func TestResolveRemoteNewer(t *testing.T) {
	cd := NewConflictDetector()
	r := cd.Resolve(
		FileVersion{Hash: "local", LamportVersion: 3, Timestamp: 1000, PeerID: "A"},
		FileVersion{Hash: "remote", LamportVersion: 5, Timestamp: 1000, PeerID: "B"},
	)
	if r.Action != AcceptRemote {
		t.Errorf("action = %v, want AcceptRemote", r.Action)
	}
	if r.IsConflict {
		t.Error("remote strictly newer should not be a conflict")
	}
}

func TestResolveLocalNewer(t *testing.T) {
	cd := NewConflictDetector()
	r := cd.Resolve(
		FileVersion{Hash: "local", LamportVersion: 7, Timestamp: 1000, PeerID: "A"},
		FileVersion{Hash: "remote", LamportVersion: 3, Timestamp: 2000, PeerID: "B"},
	)
	if r.Action != KeepLocal {
		t.Errorf("action = %v, want KeepLocal", r.Action)
	}
	if r.IsConflict {
		t.Error("local strictly newer should not be a conflict")
	}
}

func TestResolveConcurrentRemoteTimestampWins(t *testing.T) {
	cd := NewConflictDetector()
	r := cd.Resolve(
		FileVersion{Hash: "local", LamportVersion: 5, Timestamp: 1000, PeerID: "A"},
		FileVersion{Hash: "remote", LamportVersion: 5, Timestamp: 2000, PeerID: "B"},
	)
	if r.Action != AcceptRemote {
		t.Errorf("action = %v, want AcceptRemote (remote timestamp wins)", r.Action)
	}
	if !r.IsConflict {
		t.Error("concurrent edit should be marked as conflict")
	}
}

func TestResolveConcurrentLocalTimestampWins(t *testing.T) {
	cd := NewConflictDetector()
	r := cd.Resolve(
		FileVersion{Hash: "local", LamportVersion: 5, Timestamp: 3000, PeerID: "A"},
		FileVersion{Hash: "remote", LamportVersion: 5, Timestamp: 1000, PeerID: "B"},
	)
	if r.Action != KeepLocal {
		t.Errorf("action = %v, want KeepLocal (local timestamp wins)", r.Action)
	}
	if !r.IsConflict {
		t.Error("concurrent edit should be marked as conflict")
	}
}

func TestResolveConcurrentSameTimestampPeerIDTiebreak(t *testing.T) {
	cd := NewConflictDetector()

	// Same version, same timestamp — peer ID "B" > "A" lexicographically.
	r := cd.Resolve(
		FileVersion{Hash: "local", LamportVersion: 5, Timestamp: 1000, PeerID: "A"},
		FileVersion{Hash: "remote", LamportVersion: 5, Timestamp: 1000, PeerID: "B"},
	)
	if r.Action != AcceptRemote {
		t.Errorf("action = %v, want AcceptRemote (peer B > A)", r.Action)
	}
	if !r.IsConflict {
		t.Error("concurrent edit should be marked as conflict")
	}

	// Reverse: local peer "Z" > remote peer "A".
	r2 := cd.Resolve(
		FileVersion{Hash: "local", LamportVersion: 5, Timestamp: 1000, PeerID: "Z"},
		FileVersion{Hash: "remote", LamportVersion: 5, Timestamp: 1000, PeerID: "A"},
	)
	if r2.Action != KeepLocal {
		t.Errorf("action = %v, want KeepLocal (peer Z > A)", r2.Action)
	}
}

func TestResolveConcurrentSamePeerID(t *testing.T) {
	cd := NewConflictDetector()

	// Edge case: same peer ID, same version, same timestamp, different hash.
	// Local peer >= remote peer, so KeepLocal.
	r := cd.Resolve(
		FileVersion{Hash: "hash-a", LamportVersion: 5, Timestamp: 1000, PeerID: "A"},
		FileVersion{Hash: "hash-b", LamportVersion: 5, Timestamp: 1000, PeerID: "A"},
	)
	if r.Action != KeepLocal {
		t.Errorf("action = %v, want KeepLocal (same peer, >=)", r.Action)
	}
}
