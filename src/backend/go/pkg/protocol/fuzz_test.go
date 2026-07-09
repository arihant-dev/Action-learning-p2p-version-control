package protocol

import (
	"encoding/json"
	"testing"
)

func FuzzParseMessage(f *testing.F) {
	seeds := []string{
		`{"version":"1.0","type":"ping","id":"msg_123","timestamp":1704067200000,"source":"go","payload":{}}`,
		`invalid json`,
		`{"version":"1.0","type":`,
		`{"version":"1.0","type":"file_changed","payload":{"action":"add","path":"/test.txt","hash":"abc","size":100,"modified_time":12345}}`,
		`{"version":"1.0","type":"metadata_exchange","payload":{"files":[]}}`,
		`{"type":"test"}`,
		``,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	// Add a very long payload
	longPayload := make([]byte, 10000)
	for i := range longPayload {
		longPayload[i] = byte('A' + i%26)
	}
	f.Add(longPayload)

	f.Fuzz(func(t *testing.T, data []byte) {
		var msg map[string]interface{}
		err := json.Unmarshal(data, &msg)
		if err == nil {
			serialized, err := json.Marshal(msg)
			if err != nil {
				t.Errorf("valid message failed to serialize: %v", err)
			}
			var reparsed map[string]interface{}
			err = json.Unmarshal(serialized, &reparsed)
			if err != nil {
				t.Errorf("round-trip parse failed: %v", err)
			}
		}
	})
}

func FuzzFileChangedPayload(f *testing.F) {
	seeds := []string{
		`{"action":"add","path":"/foo.txt","hash":"abc123","size":100,"modified_time":12345}`,
		`{"action":"delete","path":"/foo.txt","modified_time":12345}`,
		`{"action":"modify","path":"/bar.txt","hash":"def456","size":-1,"modified_time":12345}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var p FileChangedPayload
		if err := json.Unmarshal(data, &p); err != nil {
			return
		}
		_ = p.Validate()
	})
}

func FuzzMetadataExchangePayload(f *testing.F) {
	seeds := []string{
		`{"files":[{"path":"a.txt","hash":"h1","size":100,"modified_time":1000,"vector_clock":{"A":1}}]}`,
		`{"files":[]}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var p MetadataExchangePayload
		if err := json.Unmarshal(data, &p); err != nil {
			return
		}
		_ = p.Validate()
	})
}
