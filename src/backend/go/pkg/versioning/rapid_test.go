package versioning

import (
	"testing"

	"pgregory.net/rapid"
)

func TestVectorClockConvergence(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		peers := []string{"A", "B", "C", "D"}
		vc1 := NewVectorClock()
		vc2 := NewVectorClock()
		vc3 := NewVectorClock()

		genClock := rapid.MapOf(
			rapid.SampledFrom(peers),
			rapid.Uint64Range(0, 100),
		)

		clockA := genClock.Draw(t, "clockA")
		clockB := genClock.Draw(t, "clockB")
		clockC := genClock.Draw(t, "clockC")

		vc1.Merge(clockA)
		vc1.Merge(clockB)
		vc1.Merge(clockC)

		vc2.Merge(clockA)
		vc2.Merge(clockC)
		vc2.Merge(clockB)

		vc3.Merge(clockC)
		vc3.Merge(clockB)
		vc3.Merge(clockA)

		m1 := vc1.AsMap()
		m2 := vc2.AsMap()
		m3 := vc3.AsMap()

		for k, v := range m1 {
			if m2[k] != v || m3[k] != v {
				t.Fatalf("convergence failed for key %s: m1=%d m2=%d m3=%d", k, v, m2[k], m3[k])
			}
		}
		for k, v := range m2 {
			if m1[k] != v || m3[k] != v {
				t.Fatalf("convergence failed for key %s", k)
			}
		}
	})
}

func TestVectorClockMergeIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		peers := []string{"X", "Y", "Z"}
		vc := NewVectorClock()

		genClock := rapid.MapOf(
			rapid.SampledFrom(peers),
			rapid.Uint64Range(0, 50),
		)

		clock1 := genClock.Draw(t, "clock1")
		clock2 := genClock.Draw(t, "clock2")

		vc.Merge(clock1)
		state1 := vc.AsMap()

		vc.Merge(clock2)

		vc.Merge(clock1)
		state2 := vc.AsMap()

		for k, v := range state1 {
			if state2[k] < v {
				t.Fatalf("merge not idempotent: key %s dropped from %d to %d", k, v, state2[k])
			}
		}
	})
}

func TestConflictDetectorDeterminism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cd := NewConflictDetector()

		genVersion := rapid.Custom(func(t *rapid.T) FileVersion {
			return FileVersion{
				Hash:           rapid.StringMatching("hash_[0-9a-f]{8}").Draw(t, "hash"),
				LamportVersion: rapid.Uint64Range(0, 100).Draw(t, "lamport"),
				Timestamp:      rapid.Int64Range(0, 1000000).Draw(t, "timestamp"),
				PeerID:         rapid.SampledFrom([]string{"peer-A", "peer-B", "peer-C"}).Draw(t, "peerID"),
			}
		})

		local := genVersion.Draw(t, "local")
		remote := genVersion.Draw(t, "remote")

		r1 := cd.Resolve(local, remote)
		r2 := cd.Resolve(local, remote)

		if r1.Action != r2.Action {
			t.Fatalf("non-deterministic: first=%v second=%v", r1, r2)
		}
		if r1.IsConflict != r2.IsConflict {
			t.Fatalf("non-deterministic conflict: first=%v second=%v", r1, r2)
		}
	})
}

func TestLamportClockMonotonic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		c := NewLamportClock()

		ops := rapid.SliceOf(
			rapid.SampledFrom([]string{"tick", "witness"}),
		).Draw(t, "operations")

		prev := uint64(0)
		for _, op := range ops {
			var cur uint64
			switch op {
			case "tick":
				cur = c.Tick()
			case "witness":
				remote := rapid.Uint64Range(0, 200).Draw(t, "remote")
				cur = c.Witness(remote)
			}
			if cur < prev {
				t.Fatalf("clock decreased: prev=%d cur=%d", prev, cur)
			}
			if cur == 0 {
				t.Fatal("clock should never be 0 after operation")
			}
			prev = cur
		}
	})
}

func TestVectorClockCompareTransitive(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		peers := []string{"A", "B"}

		genClock := rapid.Custom(func(t *rapid.T) *VectorClock {
			m := rapid.MapOf(
				rapid.SampledFrom(peers),
				rapid.Uint64Range(0, 20),
			).Draw(t, "m")
			return NewVectorClockFrom(m)
		})

		a := genClock.Draw(t, "a")
		b := genClock.Draw(t, "b")

		ab := a.Compare(b)
		ba := b.Compare(a)

		if ab == Equal && ba != Equal {
			t.Fatal("Equal should be symmetric")
		}
		if ab == Before && ba != After {
			t.Fatal("Before/After should be symmetric inverses")
		}
		if ab == After && ba != Before {
			t.Fatal("After/Before should be symmetric inverses")
		}
		if ab == Concurrent && ba != Concurrent {
			t.Fatal("Concurrent should be symmetric")
		}
	})
}
