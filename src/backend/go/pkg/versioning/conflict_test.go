package versioning

import (
	"testing"
)

func TestConflictDetectorIdenticalFiles(t *testing.T) {
	cd := NewConflictDetector()

	local := FileVersion{Hash: "abc", LamportVersion: 1, Timestamp: 100, PeerID: "peer-a"}
	remote := FileVersion{Hash: "abc", LamportVersion: 2, Timestamp: 200, PeerID: "peer-b"}

	res := cd.Resolve(local, remote)

	if res.Action != Skip {
		t.Errorf("expected Skip for identical hashes, got %s", res.Action)
	}
	if res.IsConflict {
		t.Error("expected IsConflict=false for identical files")
	}
}

func TestConflictDetectorRemoteNewer(t *testing.T) {
	cd := NewConflictDetector()

	local := FileVersion{Hash: "abc", LamportVersion: 1, Timestamp: 100, PeerID: "peer-a"}
	remote := FileVersion{Hash: "xyz", LamportVersion: 5, Timestamp: 200, PeerID: "peer-b"}

	res := cd.Resolve(local, remote)

	if res.Action != AcceptRemote {
		t.Errorf("expected AcceptRemote, got %s", res.Action)
	}
	if res.IsConflict {
		t.Error("expected IsConflict=false for non-conflicting newer remote")
	}
}

func TestConflictDetectorLocalNewer(t *testing.T) {
	cd := NewConflictDetector()

	local := FileVersion{Hash: "abc", LamportVersion: 10, Timestamp: 100, PeerID: "peer-a"}
	remote := FileVersion{Hash: "xyz", LamportVersion: 5, Timestamp: 200, PeerID: "peer-b"}

	res := cd.Resolve(local, remote)

	if res.Action != KeepLocal {
		t.Errorf("expected KeepLocal, got %s", res.Action)
	}
	if res.IsConflict {
		t.Error("expected IsConflict=false for non-conflicting newer local")
	}
}

func TestConflictDetectorConcurrentEditRemoteWins(t *testing.T) {
	cd := NewConflictDetector()

	local := FileVersion{Hash: "abc", LamportVersion: 5, Timestamp: 100, PeerID: "peer-a"}
	remote := FileVersion{Hash: "xyz", LamportVersion: 5, Timestamp: 200, PeerID: "peer-b"}

	res := cd.Resolve(local, remote)

	if res.Action != AcceptRemote {
		t.Errorf("expected AcceptRemote (later timestamp wins), got %s", res.Action)
	}
	if !res.IsConflict {
		t.Error("expected IsConflict=true for concurrent edit")
	}
}

func TestConflictDetectorConcurrentEditLocalWins(t *testing.T) {
	cd := NewConflictDetector()

	local := FileVersion{Hash: "abc", LamportVersion: 5, Timestamp: 300, PeerID: "peer-a"}
	remote := FileVersion{Hash: "xyz", LamportVersion: 5, Timestamp: 200, PeerID: "peer-b"}

	res := cd.Resolve(local, remote)

	if res.Action != KeepLocal {
		t.Errorf("expected KeepLocal (later timestamp wins), got %s", res.Action)
	}
	if !res.IsConflict {
		t.Error("expected IsConflict=true for concurrent edit")
	}
}

func TestConflictDetectorSameTimestampPeerIDTiebreak(t *testing.T) {
	cd := NewConflictDetector()

	local := FileVersion{Hash: "abc", LamportVersion: 5, Timestamp: 200, PeerID: "peer-a"}
	remote := FileVersion{Hash: "xyz", LamportVersion: 5, Timestamp: 200, PeerID: "peer-b"}

	res := cd.Resolve(local, remote)

	if res.Action != AcceptRemote {
		t.Errorf("expected AcceptRemote (peer-b > peer-a), got %s", res.Action)
	}
	if !res.IsConflict {
		t.Error("expected IsConflict=true for concurrent edit with same timestamp")
	}
}

func TestConflictDetectorSameTimestampPeerIDTiebreakLocalWins(t *testing.T) {
	cd := NewConflictDetector()

	local := FileVersion{Hash: "abc", LamportVersion: 5, Timestamp: 200, PeerID: "peer-z"}
	remote := FileVersion{Hash: "xyz", LamportVersion: 5, Timestamp: 200, PeerID: "peer-a"}

	res := cd.Resolve(local, remote)

	if res.Action != KeepLocal {
		t.Errorf("expected KeepLocal (peer-z > peer-a), got %s", res.Action)
	}
}

func TestActionString(t *testing.T) {
	if AcceptRemote.String() != "AcceptRemote" {
		t.Errorf("expected 'AcceptRemote', got '%s'", AcceptRemote.String())
	}
	if KeepLocal.String() != "KeepLocal" {
		t.Errorf("expected 'KeepLocal', got '%s'", KeepLocal.String())
	}
	if Skip.String() != "Skip" {
		t.Errorf("expected 'Skip', got '%s'", Skip.String())
	}
	if Action(99).String() != "Unknown" {
		t.Errorf("expected 'Unknown', got '%s'", Action(99).String())
	}
}
