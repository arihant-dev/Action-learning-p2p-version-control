package versioning

import (
	"encoding/json"
	"testing"
)

func TestLamportClockTick(t *testing.T) {
	clock := NewLamportClock()

	v1 := clock.Tick()
	if v1 != 1 {
		t.Errorf("expected first tick = 1, got %d", v1)
	}

	v2 := clock.Tick()
	if v2 != 2 {
		t.Errorf("expected second tick = 2, got %d", v2)
	}
}

func TestLamportClockWitness(t *testing.T) {
	clock := NewLamportClock()

	clock.Tick()
	clock.Tick()

	v := clock.Witness(10)
	if v != 11 {
		t.Errorf("expected witness(10) when local=2 -> max(2,10)+1=11, got %d", v)
	}

	v = clock.Witness(5)
	if v != 12 {
		t.Errorf("expected witness(5) when local=11 -> 11+1=12, got %d", v)
	}
}

func TestLamportClockValue(t *testing.T) {
	clock := NewLamportClockAt(42)
	if clock.Value() != 42 {
		t.Errorf("expected value 42, got %d", clock.Value())
	}

	clock.Tick()
	if clock.Value() != 43 {
		t.Errorf("expected value 43 after tick, got %d", clock.Value())
	}
}

func TestLamportClockMarshalJSON(t *testing.T) {
	clock := NewLamportClockAt(99)
	data, err := json.Marshal(clock)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var got uint64
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if got != 99 {
		t.Errorf("expected 99, got %d", got)
	}
}

func TestLamportClockUnmarshalJSON(t *testing.T) {
	clock := NewLamportClock()

	err := json.Unmarshal([]byte("42"), clock)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if clock.Value() != 42 {
		t.Errorf("expected value 42, got %d", clock.Value())
	}
}

func TestLamportClockConcurrent(t *testing.T) {
	clock := NewLamportClock()
	done := make(chan struct{})

	for i := 0; i < 100; i++ {
		go func() {
			clock.Tick()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	if clock.Value() != 100 {
		t.Errorf("expected value 100 after 100 concurrent ticks, got %d", clock.Value())
	}
}
