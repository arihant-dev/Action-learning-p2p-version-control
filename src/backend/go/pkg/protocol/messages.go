package protocol

import (
	"errors"
	"fmt"
)

// ============================================================================
// IPC Messages (between C++ daemon and Go coordinator)
// ============================================================================

// FileChangedPayload represents a file system change event detected by the C++ watcher.
type FileChangedPayload struct {
	Action       string `json:"action"` // "add", "modify", "delete"
	Path         string `json:"path"`
	Hash         string `json:"hash"`
	Size         int64  `json:"size"`
	ModifiedTime int64  `json:"modified_time"` // Unix timestamp in seconds
}

// Validate checks if the FileChangedPayload fields are correct.
func (p *FileChangedPayload) Validate() error {
	if p.Path == "" {
		return errors.New("path cannot be empty")
	}
	switch p.Action {
	case "add", "modify":
		if p.Hash == "" {
			return fmt.Errorf("hash cannot be empty for action: %s", p.Action)
		}
		if p.Size < 0 {
			return fmt.Errorf("invalid file size %d for action: %s", p.Size, p.Action)
		}
	case "delete":
		// Hash and Size can be empty/zero on delete
	default:
		return fmt.Errorf("unknown action type: %q", p.Action)
	}
	if p.ModifiedTime <= 0 {
		return errors.New("modified_time must be greater than 0")
	}
	return nil
}

// SyncFromPeerPayload instructs C++ daemon to apply a file retrieved from the P2P network.
type SyncFromPeerPayload struct {
	PeerID        string `json:"peer_id"`
	PeerName      string `json:"peer_name"`
	Path          string `json:"path"`
	ContentBase64 string `json:"content_base64"`
	Hash          string `json:"hash"`
	Timestamp     int64  `json:"timestamp"` // Unix timestamp in seconds
}

// Validate checks if the SyncFromPeerPayload fields are correct.
func (p *SyncFromPeerPayload) Validate() error {
	if p.PeerID == "" {
		return errors.New("peer_id cannot be empty")
	}
	if p.Path == "" {
		return errors.New("path cannot be empty")
	}
	if p.Hash == "" {
		return errors.New("hash cannot be empty")
	}
	if p.Timestamp <= 0 {
		return errors.New("timestamp must be greater than 0")
	}
	return nil
}

// PeerInfo holds connection and discovery metadata of a peer.
type PeerInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Port      int    `json:"port"`
	Connected bool   `json:"connected"`
}

// PeerListPayload contains the list of known peers to notify C++.
type PeerListPayload struct {
	Peers []PeerInfo `json:"peers"`
}

// Validate checks if the PeerListPayload fields are correct.
func (p *PeerListPayload) Validate() error {
	for i, peer := range p.Peers {
		if peer.ID == "" {
			return fmt.Errorf("peer at index %d has empty ID", i)
		}
		if peer.Address == "" {
			return fmt.Errorf("peer %s has empty address", peer.ID)
		}
		if peer.Port <= 0 || peer.Port > 65535 {
			return fmt.Errorf("peer %s has invalid port: %d", peer.ID, peer.Port)
		}
	}
	return nil
}

// VersionMetadata describes a specific version of a file in the vector clock history.
type VersionMetadata struct {
	Hash        string            `json:"hash"`
	Timestamp   int64             `json:"timestamp"`
	VectorClock map[string]uint64 `json:"vector_clock"`
	SourcePeer  string            `json:"source_peer"`
}

// ConflictDetectedPayload signals that concurrent edits on a file were detected.
type ConflictDetectedPayload struct {
	Path     string            `json:"path"`
	Versions []VersionMetadata `json:"versions"`
}

// Validate checks if the ConflictDetectedPayload fields are correct.
func (p *ConflictDetectedPayload) Validate() error {
	if p.Path == "" {
		return errors.New("path cannot be empty")
	}
	if len(p.Versions) < 2 {
		return fmt.Errorf("conflict must involve at least 2 versions, got: %d", len(p.Versions))
	}
	for i, v := range p.Versions {
		if v.Hash == "" {
			return fmt.Errorf("version at index %d has empty hash", i)
		}
		if v.Timestamp <= 0 {
			return fmt.Errorf("version at index %d has invalid timestamp: %d", i, v.Timestamp)
		}
		if v.VectorClock == nil {
			return fmt.Errorf("version at index %d has nil vector clock", i)
		}
	}
	return nil
}

// ResolutionAppliedPayload signals Go that the C++ daemon/user has resolved a conflict.
type ResolutionAppliedPayload struct {
	Path       string `json:"path"`
	ResolvedTo string `json:"resolved_to"` // "local", "remote", "merged"
	NewHash    string `json:"new_hash"`
	NewSize    int64  `json:"new_size"`
}

// Validate checks if the ResolutionAppliedPayload fields are correct.
func (p *ResolutionAppliedPayload) Validate() error {
	if p.Path == "" {
		return errors.New("path cannot be empty")
	}
	switch p.ResolvedTo {
	case "local", "remote", "merged":
		// Valid options
	default:
		return fmt.Errorf("invalid resolved_to value: %q", p.ResolvedTo)
	}
	if p.ResolvedTo == "merged" && p.NewHash == "" {
		return errors.New("new_hash cannot be empty for merged resolution")
	}
	if p.NewSize < 0 {
		return errors.New("new_size cannot be negative")
	}
	return nil
}

// PrepareFileTransferPayload initiates a port-handover for streaming large files.
type PrepareFileTransferPayload struct {
	TransferID   string `json:"transfer_id"`
	Path         string `json:"path"`
	PeerID       string `json:"peer_id"`
	TransferPort int    `json:"transfer_port"`
	ExpectedHash string `json:"expected_hash"`
	ExpectedSize int64  `json:"expected_size"`
}

// Validate checks if the PrepareFileTransferPayload fields are correct.
func (p *PrepareFileTransferPayload) Validate() error {
	if p.TransferID == "" {
		return errors.New("transfer_id cannot be empty")
	}
	if p.Path == "" {
		return errors.New("path cannot be empty")
	}
	if p.PeerID == "" {
		return errors.New("peer_id cannot be empty")
	}
	if p.TransferPort <= 0 || p.TransferPort > 65535 {
		return fmt.Errorf("invalid transfer port: %d", p.TransferPort)
	}
	if p.ExpectedHash == "" {
		return errors.New("expected_hash cannot be empty")
	}
	if p.ExpectedSize < 0 {
		return errors.New("expected_size cannot be negative")
	}
	return nil
}

// FileTransferCompletePayload notifies C++ of transfer completion status.
type FileTransferCompletePayload struct {
	TransferID string `json:"transfer_id"`
	Path       string `json:"path"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

// Validate checks if the FileTransferCompletePayload fields are correct.
func (p *FileTransferCompletePayload) Validate() error {
	if p.TransferID == "" {
		return errors.New("transfer_id cannot be empty")
	}
	if p.Path == "" {
		return errors.New("path cannot be empty")
	}
	if !p.Success && p.Error == "" {
		return errors.New("error message must be provided on failure")
	}
	return nil
}

// ============================================================================
// P2P Network Messages (between Go network coordinators)
// ============================================================================

// FileMetadata holds info about a local file version.
type FileMetadata struct {
	Path         string            `json:"path"`
	Hash         string            `json:"hash"`
	Size         int64             `json:"size"`
	ModifiedTime int64             `json:"modified_time"`
	VectorClock  map[string]uint64 `json:"vector_clock"`
}

// MetadataExchangePayload represents the local index shared between peers on sync.
type MetadataExchangePayload struct {
	Files []FileMetadata `json:"files"`
}

// Validate checks if the MetadataExchangePayload fields are correct.
func (p *MetadataExchangePayload) Validate() error {
	for i, f := range p.Files {
		if f.Path == "" {
			return fmt.Errorf("file at index %d has empty path", i)
		}
		if f.Hash == "" {
			return fmt.Errorf("file %s has empty hash", f.Path)
		}
		if f.Size < 0 {
			return fmt.Errorf("file %s has invalid size: %d", f.Path, f.Size)
		}
		if f.ModifiedTime <= 0 {
			return fmt.Errorf("file %s has invalid modified_time: %d", f.Path, f.ModifiedTime)
		}
		if f.VectorClock == nil {
			return fmt.Errorf("file %s has nil vector clock", f.Path)
		}
	}
	return nil
}

// FileRequestPayload requests a file's content from a remote peer.
type FileRequestPayload struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// Validate checks if the FileRequestPayload fields are correct.
func (p *FileRequestPayload) Validate() error {
	if p.Path == "" {
		return errors.New("path cannot be empty")
	}
	if p.Hash == "" {
		return errors.New("hash cannot be empty")
	}
	return nil
}

// FileResponsePayload returns file data (for small files) or a port (for socket handover).
type FileResponsePayload struct {
	Path          string `json:"path"`
	Hash          string `json:"hash"`
	ContentBase64 string `json:"content_base64,omitempty"`
	TransferPort  int    `json:"transfer_port,omitempty"`
	Error         string `json:"error,omitempty"`
}

// Validate checks if the FileResponsePayload fields are correct.
func (p *FileResponsePayload) Validate() error {
	if p.Path == "" {
		return errors.New("path cannot be empty")
	}
	if p.Error != "" {
		return nil // Invalid parameters are okay if returning an error
	}
	if p.Hash == "" {
		return errors.New("hash cannot be empty on successful response")
	}
	if p.ContentBase64 == "" && p.TransferPort <= 0 {
		return errors.New("either content_base64 or transfer_port must be populated")
	}
	if p.TransferPort > 65535 {
		return fmt.Errorf("invalid transfer port: %d", p.TransferPort)
	}
	return nil
}

// AcknowledgmentPayload confirms receipt of a sync action or message.
type AcknowledgmentPayload struct {
	MessageID string `json:"message_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// Validate checks if the AcknowledgmentPayload fields are correct.
func (p *AcknowledgmentPayload) Validate() error {
	if p.MessageID == "" {
		return errors.New("message_id cannot be empty")
	}
	if !p.Success && p.Error == "" {
		return errors.New("error message must be provided on failure")
	}
	return nil
}
