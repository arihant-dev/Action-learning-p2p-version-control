package versioning

import "fmt"

// Action describes what the sync engine should do with a file after
// conflict resolution.
type Action int

const (
	// AcceptRemote means the remote version wins; overwrite the local file
	// (after backing up the local version to .versions/).
	AcceptRemote Action = iota
	// KeepLocal means the local version wins; ignore the incoming change.
	KeepLocal
	// Skip means the files are identical; no action is needed.
	Skip
)

// String returns a human-readable label.
func (a Action) String() string {
	switch a {
	case AcceptRemote:
		return "AcceptRemote"
	case KeepLocal:
		return "KeepLocal"
	case Skip:
		return "Skip"
	default:
		return "Unknown"
	}
}

// Resolution is the output of the conflict resolver. It tells the sync
// engine what to do and why.
type Resolution struct {
	Action     Action
	Reason     string
	IsConflict bool // True when a real concurrent edit was detected.
}

// FileVersion describes a specific version of a file, carrying enough
// information for the conflict detector to decide who wins.
type FileVersion struct {
	Hash           string
	LamportVersion uint64 // Lamport clock value at the time of the edit
	Timestamp      int64  // Physical wall-clock time (Unix ms), used as tie-breaker
	PeerID         string // Which peer produced this version
}

// ConflictDetector implements the Last-Write-Wins (LWW) conflict
// resolution policy described in the thesis. When two versions have
// the same Lamport clock value, the physical timestamp is used as a
// tie-breaker. When timestamps also collide, the lexicographically
// greater peer ID wins (deterministic tie-break so all nodes agree).
type ConflictDetector struct{}

// NewConflictDetector returns a new detector.
func NewConflictDetector() *ConflictDetector {
	return &ConflictDetector{}
}

// Resolve compares a local and remote file version and returns a
// Resolution indicating the action the sync engine should take.
func (cd *ConflictDetector) Resolve(local, remote FileVersion) Resolution {
	// Fast path: identical content — nothing to do.
	if local.Hash == remote.Hash {
		return Resolution{
			Action: Skip,
			Reason: "files are identical (same hash)",
		}
	}

	// Compare Lamport versions.
	switch {
	case remote.LamportVersion > local.LamportVersion:
		// Remote is strictly newer — accept it.
		return Resolution{
			Action:     AcceptRemote,
			Reason:     fmt.Sprintf("remote version %d > local version %d", remote.LamportVersion, local.LamportVersion),
			IsConflict: false,
		}

	case local.LamportVersion > remote.LamportVersion:
		// Local is strictly newer — keep it.
		return Resolution{
			Action:     KeepLocal,
			Reason:     fmt.Sprintf("local version %d > remote version %d", local.LamportVersion, remote.LamportVersion),
			IsConflict: false,
		}

	default:
		// Same Lamport version but different hashes → concurrent edit.
		// Use physical timestamp as tie-breaker (LWW).
		return cd.resolveConcurrent(local, remote)
	}
}

// resolveConcurrent handles the case where two versions have the same
// Lamport clock value but different content.
func (cd *ConflictDetector) resolveConcurrent(local, remote FileVersion) Resolution {
	isConflict := true

	switch {
	case remote.Timestamp > local.Timestamp:
		return Resolution{
			Action:     AcceptRemote,
			Reason:     fmt.Sprintf("concurrent edit: remote timestamp %d > local %d (LWW)", remote.Timestamp, local.Timestamp),
			IsConflict: isConflict,
		}

	case local.Timestamp > remote.Timestamp:
		return Resolution{
			Action:     KeepLocal,
			Reason:     fmt.Sprintf("concurrent edit: local timestamp %d > remote %d (LWW)", local.Timestamp, remote.Timestamp),
			IsConflict: isConflict,
		}

	default:
		// Timestamps are also identical — deterministic tie-break by peer ID.
		if remote.PeerID > local.PeerID {
			return Resolution{
				Action:     AcceptRemote,
				Reason:     fmt.Sprintf("concurrent edit: same timestamp, remote peer %q > local peer %q (deterministic tie-break)", remote.PeerID, local.PeerID),
				IsConflict: isConflict,
			}
		}
		return Resolution{
			Action:     KeepLocal,
			Reason:     fmt.Sprintf("concurrent edit: same timestamp, local peer %q >= remote peer %q (deterministic tie-break)", local.PeerID, remote.PeerID),
			IsConflict: isConflict,
		}
	}
}
