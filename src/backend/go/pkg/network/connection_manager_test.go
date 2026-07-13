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
	cmA.SetOnConnected(func(peerID string) {
		connectedA <- peerID
	})

	connectedB := make(chan string, 1)
	cmB.SetOnConnected(func(peerID string) {
		connectedB <- peerID
	})

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
	cmReceiver.SetOnConnected(func(peerID string) {
		connectedChan <- true
	})

	err = cmSender.Connect("receiver", "127.0.0.1", addr.Port)
	if err != nil {
		t.Fatalf("failed to connect sender: %v", err)
	}

	<-connectedChan

	// Register OnMessage listener on receiver
	receivedMsgChan := make(chan *ipc.Message, 1)
	cmReceiver.SetOnMessage(func(peerID string, msg *ipc.Message) {
		if peerID == "sender" {
			receivedMsgChan <- msg
		}
	})

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

func TestHeartbeatTimeout(t *testing.T) {
	cmA := NewConnectionManager("peer-a")
	cmA.heartbeatInterval = 50 * time.Millisecond
	cmA.heartbeatTimeout = 150 * time.Millisecond
	err := cmA.StartServer(0)
	if err != nil {
		t.Fatalf("failed to start server A: %v", err)
	}
	defer cmA.Stop()

	// Start a raw TCP server to simulate a frozen peer B
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	disconnectedChan := make(chan string, 1)
	cmA.SetOnDisconnected(func(peerID string) {
		disconnectedChan <- peerID
	})

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read handshake
		msg, err := ipc.ReadMessage(conn)
		if err != nil {
			return
		}
		if msg.Type != "handshake" {
			return
		}

		// Write handshake response
		respPayload, _ := json.Marshal(HandshakePayload{PeerID: "peer-b"})
		respMsg := &ipc.Message{
			Version: "1.0",
			Type:    "handshake",
			Payload: respPayload,
		}
		_ = ipc.WriteMessage(conn, respMsg)

		// Sleep and ignore all incoming pings
		time.Sleep(1 * time.Second)
	}()

	err = cmA.Connect("peer-b", "127.0.0.1", port)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	if !cmA.IsConnected("peer-b") {
		t.Fatal("expected peer-b to be connected initially")
	}

	select {
	case id := <-disconnectedChan:
		if id != "peer-b" {
			t.Errorf("expected disconnected peer-b, got %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for heartbeat disconnect")
	}

	if cmA.IsConnected("peer-b") {
		t.Error("expected peer-b to be disconnected after heartbeat timeout")
	}
}

func TestAutoReconnect(t *testing.T) {
	// Server A
	cmA := NewConnectionManager("peer-a")
	err := cmA.StartServer(0)
	if err != nil {
		t.Fatalf("failed to start server A: %v", err)
	}
	defer cmA.Stop()

	cmA.mu.RLock()
	addr := cmA.listener.Addr().(*net.TCPAddr)
	cmA.mu.RUnlock()

	// Client B
	cmB := NewConnectionManager("peer-b")
	cmB.reconnectInterval = 100 * time.Millisecond
	err = cmB.StartServer(0)
	if err != nil {
		t.Fatalf("failed to start server B: %v", err)
	}
	defer cmB.Stop()

	connectedChan := make(chan string, 2)
	cmB.SetOnConnected(func(peerID string) {
		connectedChan <- peerID
	})

	err = cmB.Connect("peer-a", "127.0.0.1", addr.Port)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Verify initial connection
	select {
	case id := <-connectedChan:
		if id != "peer-a" {
			t.Errorf("expected connection to peer-a, got %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for initial connection")
	}

	// Set disconnect listener BEFORE stopping Server A to avoid missing the signal
	disconnected := make(chan struct{}, 1)
	cmB.SetOnDisconnected(func(peerID string) {
		if peerID == "peer-a" {
			select {
			case disconnected <- struct{}{}:
			default:
			}
		}
	})

	// Disconnect Server A
	cmA.Stop()

	// Wait for client B to detect disconnect — use polling loop to tolerate
	// goroutine scheduling delays on busy CI runners.
	deadline := time.Now().Add(5 * time.Second)
	detected := false
	for time.Now().Before(deadline) {
		select {
		case <-disconnected:
			detected = true
		default:
		}
		if detected || !cmB.IsConnected("peer-a") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !detected && cmB.IsConnected("peer-a") {
		t.Fatal("timeout waiting for B to detect disconnect of A")
	}

	// Restart Server A on same port
	cmA2 := NewConnectionManager("peer-a")
	err = cmA2.StartServer(addr.Port)
	if err != nil {
		t.Fatalf("failed to restart server A: %v", err)
	}
	defer cmA2.Stop()

	// Wait for B to reconnect automatically
	select {
	case id := <-connectedChan:
		if id != "peer-a" {
			t.Errorf("expected reconnect to peer-a, got %s", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for auto-reconnection")
	}

	if !cmB.IsConnected("peer-a") {
		t.Error("expected B to be connected to peer-a after auto-reconnect")
	}
}
