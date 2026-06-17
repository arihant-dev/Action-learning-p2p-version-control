package network

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"p2p/pkg/ipc"
)

func TestConnectionManagerHandshakeAndServer(t *testing.T) {
	// Setup manager A (Server)
	cmA := NewConnectionManager("peer-a")
	defer cmA.Stop()

	// Setup manager B (Client)
	cmB := NewConnectionManager("peer-b")
	defer cmB.Stop()

	// Start Server A on random local port (0 binds to dynamic port)
	err := cmA.StartServer(0)
	if err != nil {
		t.Fatalf("failed to start server A: %v", err)
	}

	// Find bound port
	cmA.mu.RLock()
	addr := cmA.listener.Addr().(*net.TCPAddr)
	cmA.mu.RUnlock()

	connectedA := make(chan string, 1)
	cmA.OnConnected = func(peerID string) {
		connectedA <- peerID
	}

	connectedB := make(chan string, 1)
	cmB.OnConnected = func(peerID string) {
		connectedB <- peerID
	}

	// Dial B to A
	err = cmB.Connect("peer-a", "127.0.0.1", addr.Port)
	if err != nil {
		t.Fatalf("Client B failed to connect to Server A: %v", err)
	}

	// Verify handshakes complete
	select {
	case id := <-connectedA:
		if id != "peer-b" {
			t.Errorf("expected Peer A to register peer-b, got %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Peer A connection registration")
	}

	select {
	case id := <-connectedB:
		if id != "peer-a" {
			t.Errorf("expected Peer B to register peer-a, got %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Peer B connection registration")
	}

	if !cmA.IsConnected("peer-b") {
		t.Error("expected Peer A to mark peer-b as connected")
	}
	if !cmB.IsConnected("peer-a") {
		t.Error("expected Peer B to mark peer-a as connected")
	}
}

func TestBroadcast(t *testing.T) {
	cmSender := NewConnectionManager("sender")
	defer cmSender.Stop()

	cmReceiver := NewConnectionManager("receiver")
	defer cmReceiver.Stop()

	err := cmReceiver.StartServer(0)
	if err != nil {
		t.Fatalf("failed to start receiver server: %v", err)
	}

	cmReceiver.mu.RLock()
	addr := cmReceiver.listener.Addr().(*net.TCPAddr)
	cmReceiver.mu.RUnlock()

	connectedChan := make(chan bool, 1)
	cmReceiver.OnConnected = func(peerID string) {
		connectedChan <- true
	}

	err = cmSender.Connect("receiver", "127.0.0.1", addr.Port)
	if err != nil {
		t.Fatalf("failed to connect sender: %v", err)
	}

	<-connectedChan

	// Register clean OnMessage listener on receiver
	receivedMsgChan := make(chan *ipc.Message, 1)
	cmReceiver.OnMessage = func(peerID string, msg *ipc.Message) {
		if peerID == "sender" {
			receivedMsgChan <- msg
		}
	}

	// Wait a tiny moment for registries to finalize
	time.Sleep(100 * time.Millisecond)

	// Prepare broadcast msg
	payloadMsg, _ := json.Marshal(map[string]string{"action": "broadcast"})
	msg := &ipc.Message{
		Version: "1.0",
		Type:    "file_changed",
		Payload: payloadMsg,
	}

	// Broadcast on sender
	cmSender.Broadcast(msg)

	// Read from channel
	select {
	case readMsg := <-receivedMsgChan:
		if readMsg.Type != "file_changed" {
			t.Errorf("expected broadcast type file_changed, got %s", readMsg.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting to receive broadcast message")
	}
}
