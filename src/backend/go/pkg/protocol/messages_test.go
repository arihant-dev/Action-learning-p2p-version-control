package protocol

import (
	"encoding/json"
	"testing"
)

func TestFileChangedPayloadValidation(t *testing.T) {
	tests := []struct {
		name    string
		payload FileChangedPayload
		wantErr bool
	}{
		{
			name: "valid add action",
			payload: FileChangedPayload{
				Action:       "add",
				Path:         "/foo/bar.txt",
				Hash:         "hash123",
				Size:         100,
				ModifiedTime: 1234567,
			},
			wantErr: false,
		},
		{
			name: "valid delete action with empty hash",
			payload: FileChangedPayload{
				Action:       "delete",
				Path:         "/foo/bar.txt",
				ModifiedTime: 1234567,
			},
			wantErr: false,
		},
		{
			name: "missing path",
			payload: FileChangedPayload{
				Action:       "add",
				Hash:         "hash123",
				Size:         100,
				ModifiedTime: 1234567,
			},
			wantErr: true,
		},
		{
			name: "missing hash for add",
			payload: FileChangedPayload{
				Action:       "add",
				Path:         "/foo/bar.txt",
				Size:         100,
				ModifiedTime: 1234567,
			},
			wantErr: true,
		},
		{
			name: "negative file size for modify",
			payload: FileChangedPayload{
				Action:       "modify",
				Path:         "/foo/bar.txt",
				Hash:         "hash123",
				Size:         -50,
				ModifiedTime: 1234567,
			},
			wantErr: true,
		},
		{
			name: "invalid action name",
			payload: FileChangedPayload{
				Action:       "rename",
				Path:         "/foo/bar.txt",
				Hash:         "hash123",
				Size:         100,
				ModifiedTime: 1234567,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("FileChangedPayload.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSyncFromPeerPayloadValidation(t *testing.T) {
	tests := []struct {
		name    string
		payload SyncFromPeerPayload
		wantErr bool
	}{
		{
			name: "valid sync payload",
			payload: SyncFromPeerPayload{
				PeerID:        "peer-1",
				PeerName:      "Alice",
				Path:          "file.txt",
				ContentBase64: "aGVsbG8=",
				Hash:          "hash123",
				Timestamp:     12345,
			},
			wantErr: false,
		},
		{
			name: "missing peer ID",
			payload: SyncFromPeerPayload{
				Path:      "file.txt",
				Hash:      "hash123",
				Timestamp: 12345,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SyncFromPeerPayload.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConflictDetectedPayloadValidation(t *testing.T) {
	tests := []struct {
		name    string
		payload ConflictDetectedPayload
		wantErr bool
	}{
		{
			name: "valid conflict payload",
			payload: ConflictDetectedPayload{
				Path: "conflict.txt",
				Versions: []VersionMetadata{
					{
						Hash:        "hash1",
						Timestamp:   1111,
						VectorClock: map[string]uint64{"peerA": 1},
						SourcePeer:  "peerA",
					},
					{
						Hash:        "hash2",
						Timestamp:   2222,
						VectorClock: map[string]uint64{"peerB": 1},
						SourcePeer:  "peerB",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "insufficient versions",
			payload: ConflictDetectedPayload{
				Path: "conflict.txt",
				Versions: []VersionMetadata{
					{
						Hash:        "hash1",
						Timestamp:   1111,
						VectorClock: map[string]uint64{"peerA": 1},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ConflictDetectedPayload.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original := MetadataExchangePayload{
		Files: []FileMetadata{
			{
				Path:         "a.txt",
				Hash:         "hashA",
				Size:         500,
				ModifiedTime: 1234567,
				VectorClock:  map[string]uint64{"peerA": 2, "peerB": 1},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed MetadataExchangePayload
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(parsed.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(parsed.Files))
	}

	file := parsed.Files[0]
	if file.Path != original.Files[0].Path || file.Hash != original.Files[0].Hash || file.Size != original.Files[0].Size {
		t.Errorf("unmarshaled values differ from original: %+v", file)
	}

	if file.VectorClock["peerA"] != 2 || file.VectorClock["peerB"] != 1 {
		t.Errorf("vector clock values differ: %+v", file.VectorClock)
	}
}
