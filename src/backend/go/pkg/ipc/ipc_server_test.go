package ipc

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestFramingAndReadWrite(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	payloadMsg := json.RawMessage(`{"hello":"world"}`)
	originalMsg := &Message{
		Version:   "1.0",
		Type:      "test_msg",
		ID:        "msg_123",
		Timestamp: time.Now().Unix(),
		Source:    "go",
		Payload:   payloadMsg,
	}

	// Write on side 1 in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- WriteMessage(c1, originalMsg)
	}()

	// Read on side 2
	receivedMsg, err := ReadMessage(c2)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	if err := <-errChan; err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	if receivedMsg.Version != originalMsg.Version {
		t.Errorf("expected version %s, got %s", originalMsg.Version, receivedMsg.Version)
	}
	if receivedMsg.Type != originalMsg.Type {
		t.Errorf("expected type %s, got %s", originalMsg.Type, receivedMsg.Type)
	}
	if receivedMsg.ID != originalMsg.ID {
		t.Errorf("expected ID %s, got %s", originalMsg.ID, receivedMsg.ID)
	}
	if string(receivedMsg.Payload) != string(originalMsg.Payload) {
		t.Errorf("expected payload %s, got %s", string(originalMsg.Payload), string(receivedMsg.Payload))
	}
}

func TestMessageSizeLimit(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	// Construct large dummy content
	largePayload := make([]byte, 1024*1024+10) // > 1MB
	payloadMsg, _ := json.Marshal(largePayload)
	originalMsg := &Message{
		Version: "1.0",
		Type:    "large",
		Payload: payloadMsg,
	}

	go func() {
		_ = WriteMessage(c1, originalMsg)
	}()

	_, err := ReadMessage(c2)
	if err == nil {
		t.Fatal("expected error due to size limit, got nil")
	}
}
