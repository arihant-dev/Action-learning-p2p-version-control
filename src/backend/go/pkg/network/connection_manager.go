package network

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"p2p/pkg/ipc"
)

type HandshakePayload struct {
	PeerID string `json:"peer_id"`
}

type ConnectionManager struct {
	localPeerID string
	connections map[string]net.Conn
	mu          sync.RWMutex
	listener    net.Listener
	running     bool

	OnConnected    func(peerID string)
	OnDisconnected func(peerID string)
	OnMessage      func(peerID string, msg *ipc.Message)
}

func NewConnectionManager(localPeerID string) *ConnectionManager {
	return &ConnectionManager{
		localPeerID: localPeerID,
		connections: make(map[string]net.Conn),
	}
}

// StartServer starts the TCP server to listen for incoming P2P connections
func (cm *ConnectionManager) StartServer(port int) error {
	addr := ":" + strconv.Itoa(port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start P2P server on port %d: %v", port, err)
	}

	cm.mu.Lock()
	cm.listener = listener
	cm.running = true
	cm.mu.Unlock()

	log.Printf("P2P server listening on port %d\n", port)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				cm.mu.RLock()
				stillRunning := cm.running
				cm.mu.RUnlock()
				if !stillRunning {
					return
				}
				log.Printf("Error accepting incoming P2P connection: %v\n", err)
				continue
			}

			go cm.handleIncomingConnection(conn)
		}
	}()

	return nil
}

func (cm *ConnectionManager) handleIncomingConnection(conn net.Conn) {
	// 1. Read handshake message from initiator
	msg, err := ipc.ReadMessage(conn)
	if err != nil {
		log.Printf("Failed to read handshake from incoming connection: %v\n", err)
		conn.Close()
		return
	}

	if msg.Type != "handshake" {
		log.Printf("Unexpected initial message type: %s\n", msg.Type)
		conn.Close()
		return
	}

	var payload HandshakePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Printf("Failed to unmarshal handshake payload: %v\n", err)
		conn.Close()
		return
	}

	peerID := payload.PeerID
	if peerID == "" {
		log.Printf("Received empty peer_id in handshake\n")
		conn.Close()
		return
	}

	// 2. Send handshake response back
	respPayload, _ := json.Marshal(HandshakePayload{PeerID: cm.localPeerID})
	respMsg := &ipc.Message{
		Version:   "1.0",
		Type:      "handshake",
		Source:    "go",
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		Payload:   respPayload,
	}

	if err := ipc.WriteMessage(conn, respMsg); err != nil {
		log.Printf("Failed to write handshake response to %s: %v\n", peerID, err)
		conn.Close()
		return
	}

	// 3. Register connection
	cm.mu.Lock()
	if oldConn, exists := cm.connections[peerID]; exists {
		oldConn.Close()
	}
	cm.connections[peerID] = conn
	cm.mu.Unlock()

	log.Printf("Accepted peer connection from: %s (%s)\n", peerID, conn.RemoteAddr())

	if cm.OnConnected != nil {
		go cm.OnConnected(peerID)
	}

	// 4. Start active reader loop
	go cm.readLoop(peerID, conn)
}

// Connect dials out to a discovered peer and performs the P2P handshake
func (cm *ConnectionManager) Connect(peerID, address string, port int) error {
	cm.mu.Lock()
	if _, exists := cm.connections[peerID]; exists {
		cm.mu.Unlock()
		return nil // Already connected
	}
	cm.mu.Unlock()

	addr := net.JoinHostPort(address, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %v", addr, err)
	}

	// 1. Send initiator handshake
	reqPayload, _ := json.Marshal(HandshakePayload{PeerID: cm.localPeerID})
	reqMsg := &ipc.Message{
		Version:   "1.0",
		Type:      "handshake",
		Source:    "go",
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		Payload:   reqPayload,
	}

	if err := ipc.WriteMessage(conn, reqMsg); err != nil {
		conn.Close()
		return fmt.Errorf("failed to send initiator handshake to %s: %v", peerID, err)
	}

	// 2. Read handshake response
	msg, err := ipc.ReadMessage(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to read handshake response from %s: %v", peerID, err)
	}

	if msg.Type != "handshake" {
		conn.Close()
		return fmt.Errorf("unexpected handshake response type: %s", msg.Type)
	}

	var payload HandshakePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		conn.Close()
		return fmt.Errorf("failed to unmarshal handshake response payload: %v", err)
	}

	if payload.PeerID != peerID {
		conn.Close()
		return fmt.Errorf("handshake peer_id mismatch: expected %s, got %s", peerID, payload.PeerID)
	}

	// 3. Register connection
	cm.mu.Lock()
	if oldConn, exists := cm.connections[peerID]; exists {
		oldConn.Close()
	}
	cm.connections[peerID] = conn
	cm.mu.Unlock()

	log.Printf("Successfully established P2P connection to: %s (%s)\n", peerID, addr)

	if cm.OnConnected != nil {
		go cm.OnConnected(peerID)
	}

	// 4. Start active reader loop
	go cm.readLoop(peerID, conn)

	return nil
}

func (cm *ConnectionManager) readLoop(peerID string, conn net.Conn) {
	defer cm.CloseConnection(peerID)

	for {
		msg, err := ipc.ReadMessage(conn)
		if err != nil {
			// EOF or closed connection is normal on shutdown/disconnect
			return
		}

		if cm.OnMessage != nil {
			cm.OnMessage(peerID, msg)
		}
	}
}

func (cm *ConnectionManager) GetConnection(peerID string) net.Conn {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.connections[peerID]
}

func (cm *ConnectionManager) CloseConnection(peerID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if conn, exists := cm.connections[peerID]; exists {
		conn.Close()
		delete(cm.connections, peerID)
		log.Printf("Disconnected from peer: %s\n", peerID)

		if cm.OnDisconnected != nil {
			go cm.OnDisconnected(peerID)
		}
	}
}

func (cm *ConnectionManager) IsConnected(peerID string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	_, exists := cm.connections[peerID]
	return exists
}

// Broadcast sends a message to all active TCP peer connections
func (cm *ConnectionManager) Broadcast(msg *ipc.Message) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for peerID, conn := range cm.connections {
		go func(id string, c net.Conn) {
			if err := ipc.WriteMessage(c, msg); err != nil {
				log.Printf("Failed to send message to peer %s: %v\n", id, err)
				cm.CloseConnection(id)
			}
		}(peerID, conn)
	}
}

// Stop stops the server and closes all peer connections
func (cm *ConnectionManager) Stop() {
	cm.mu.Lock()
	cm.running = false
	if cm.listener != nil {
		cm.listener.Close()
	}
	for id, conn := range cm.connections {
		conn.Close()
		delete(cm.connections, id)
	}
	cm.mu.Unlock()
}
