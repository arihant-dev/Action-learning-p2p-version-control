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

type targetPeer struct {
	id         string
	address    string
	port       int
	nextRetry  time.Time
	retryDelay time.Duration
}

type ConnectionManager struct {
	localPeerID string
	connections map[string]net.Conn
	writeMus    map[string]*sync.Mutex
	mu          sync.RWMutex
	listener    net.Listener
	running     bool

	// Heartbeat configurations
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
	lastSeen          map[string]time.Time

	// Reconnection configurations
	targets           map[string]*targetPeer
	reconnectInterval time.Duration
	maxRetryDelay     time.Duration

	// Channels and WaitGroup for loop control
	stopChan chan struct{}
	wg       sync.WaitGroup

	OnConnected    func(peerID string)
	OnDisconnected func(peerID string)
	OnMessage      func(peerID string, msg *ipc.Message)
}

func NewConnectionManager(localPeerID string) *ConnectionManager {
	return &ConnectionManager{
		localPeerID:       localPeerID,
		connections:       make(map[string]net.Conn),
		writeMus:          make(map[string]*sync.Mutex),
		lastSeen:          make(map[string]time.Time),
		targets:           make(map[string]*targetPeer),
		heartbeatInterval: 5 * time.Second,
		heartbeatTimeout:  15 * time.Second,
		reconnectInterval: 2 * time.Second,
		maxRetryDelay:     60 * time.Second,
		stopChan:          make(chan struct{}),
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

	// Start accepting connections
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

	// Start heartbeat and reconnect tasks
	cm.wg.Add(2)
	go cm.heartbeatLoop()
	go cm.reconnectLoop()

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
	cm.writeMus[peerID] = &sync.Mutex{}
	cm.lastSeen[peerID] = time.Now()
	cm.mu.Unlock()

	log.Printf("Accepted peer connection from: %s (%s)\n", peerID, conn.RemoteAddr())

	if cm.OnConnected != nil {
		go cm.OnConnected(peerID)
	}

	// 4. Start active reader loop
	go cm.readLoop(peerID, conn)
}

// Connect dials out to a discovered peer and performs the P2P handshake.
// Also registers the peer ID, address, and port in target list for auto-reconnection.
func (cm *ConnectionManager) Connect(peerID, address string, port int) error {
	cm.mu.Lock()
	// Add or update target list
	if _, exists := cm.targets[peerID]; !exists {
		cm.targets[peerID] = &targetPeer{
			id:         peerID,
			address:    address,
			port:       port,
			retryDelay: 1 * time.Second,
		}
	}
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
	cm.writeMus[peerID] = &sync.Mutex{}
	cm.lastSeen[peerID] = time.Now()
	// Reset backoff on successful connect
	if t, exists := cm.targets[peerID]; exists {
		t.retryDelay = 1 * time.Second
		t.nextRetry = time.Time{}
	}
	cm.mu.Unlock()

	log.Printf("Successfully established P2P connection to: %s (%s)\n", peerID, addr)

	if cm.OnConnected != nil {
		go cm.OnConnected(peerID)
	}

	// 4. Start active reader loop
	go cm.readLoop(peerID, conn)

	return nil
}

// RemoveTarget disconnects from a peer and removes it from the auto-reconnection target list
func (cm *ConnectionManager) RemoveTarget(peerID string) {
	cm.mu.Lock()
	delete(cm.targets, peerID)
	cm.mu.Unlock()
	cm.CloseConnection(peerID)
}

func (cm *ConnectionManager) readLoop(peerID string, conn net.Conn) {
	defer cm.CloseConnection(peerID)

	for {
		msg, err := ipc.ReadMessage(conn)
		if err != nil {
			// EOF or closed connection is normal on shutdown/disconnect
			return
		}

		// Update activity on any message received
		cm.mu.Lock()
		cm.lastSeen[peerID] = time.Now()
		cm.mu.Unlock()

		if msg.Type == "ping" {
			pongMsg := &ipc.Message{
				Version:   "1.0",
				Type:      "pong",
				Source:    "go",
				Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			}
			_ = cm.SendToPeer(peerID, pongMsg)
			continue
		}

		if msg.Type == "pong" {
			continue
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
		delete(cm.writeMus, peerID)
		delete(cm.lastSeen, peerID)
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

// SendToPeer writes a message to a specific peer's TCP connection in a thread-safe serialized way.
func (cm *ConnectionManager) SendToPeer(peerID string, msg *ipc.Message) error {
	cm.mu.RLock()
	conn, existsConn := cm.connections[peerID]
	mu, existsMu := cm.writeMus[peerID]
	cm.mu.RUnlock()

	if !existsConn || !existsMu {
		return fmt.Errorf("peer %s not connected", peerID)
	}

	mu.Lock()
	defer mu.Unlock()
	return ipc.WriteMessage(conn, msg)
}

// Broadcast sends a message to all active TCP peer connections
func (cm *ConnectionManager) Broadcast(msg *ipc.Message) {
	cm.mu.RLock()
	peerIDs := make([]string, 0, len(cm.connections))
	for id := range cm.connections {
		peerIDs = append(peerIDs, id)
	}
	cm.mu.RUnlock()

	for _, id := range peerIDs {
		go func(peerID string) {
			if err := cm.SendToPeer(peerID, msg); err != nil {
				log.Printf("Failed to broadcast message to peer %s: %v\n", peerID, err)
				cm.CloseConnection(peerID)
			}
		}(id)
	}
}

// Stop stops the server and closes all peer connections
func (cm *ConnectionManager) Stop() {
	cm.mu.Lock()
	if !cm.running {
		cm.mu.Unlock()
		return
	}
	cm.running = false
	if cm.listener != nil {
		cm.listener.Close()
	}
	for id, conn := range cm.connections {
		conn.Close()
		delete(cm.connections, id)
	}
	cm.mu.Unlock()

	close(cm.stopChan)
	cm.wg.Wait()
}

// heartbeatLoop runs periodically to send ping packets and detect timed out peers
func (cm *ConnectionManager) heartbeatLoop() {
	defer cm.wg.Done()
	ticker := time.NewTicker(cm.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopChan:
			return
		case <-ticker.C:
			cm.checkHeartbeats()
		}
	}
}

func (cm *ConnectionManager) checkHeartbeats() {
	cm.mu.Lock()
	now := time.Now()
	var toDisconnect []string

	// 1. Detect timeouts
	for id, lastSeenTime := range cm.lastSeen {
		if now.Sub(lastSeenTime) > cm.heartbeatTimeout {
			toDisconnect = append(toDisconnect, id)
		}
	}
	cm.mu.Unlock()

	// Disconnect peers that timed out
	for _, id := range toDisconnect {
		log.Printf("[ConnectionManager] Heartbeat timeout for peer %s. Disconnecting.\n", id)
		cm.CloseConnection(id)
	}

	// 2. Send ping heartbeats
	cm.mu.RLock()
	pingMsg := &ipc.Message{
		Version:   "1.0",
		Type:      "ping",
		Source:    "go",
		Timestamp: now.UnixNano() / int64(time.Millisecond),
	}
	var peerIDs []string
	for id := range cm.connections {
		peerIDs = append(peerIDs, id)
	}
	cm.mu.RUnlock()

	for _, id := range peerIDs {
		go func(peerID string) {
			if err := cm.SendToPeer(peerID, pingMsg); err != nil {
				log.Printf("[ConnectionManager] Failed to send ping to peer %s: %v. Disconnecting.\n", peerID, err)
				cm.CloseConnection(peerID)
			}
		}(id)
	}
}

// reconnectLoop attempts to dial disconnected target peers using exponential backoff
func (cm *ConnectionManager) reconnectLoop() {
	defer cm.wg.Done()
	ticker := time.NewTicker(cm.reconnectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopChan:
			return
		case <-ticker.C:
			cm.attemptReconnections()
		}
	}
}

func (cm *ConnectionManager) attemptReconnections() {
	cm.mu.Lock()
	type targetInfo struct {
		id      string
		address string
		port    int
	}
	var toRetry []targetInfo
	now := time.Now()

	for id, target := range cm.targets {
		if _, connected := cm.connections[id]; connected {
			continue
		}
		if now.Before(target.nextRetry) {
			continue
		}
		toRetry = append(toRetry, targetInfo{
			id:      target.id,
			address: target.address,
			port:    target.port,
		})
	}
	cm.mu.Unlock()

	for _, target := range toRetry {
		go func(t targetInfo) {
			log.Printf("[ConnectionManager] Auto-reconnecting to target peer %s (%s:%d)...\n", t.id, t.address, t.port)
			err := cm.Connect(t.id, t.address, t.port)

			cm.mu.Lock()
			defer cm.mu.Unlock()

			tPeer, exists := cm.targets[t.id]
			if !exists {
				return
			}

			if err != nil {
				tPeer.retryDelay *= 2
				if tPeer.retryDelay > cm.maxRetryDelay {
					tPeer.retryDelay = cm.maxRetryDelay
				}
				tPeer.nextRetry = time.Now().Add(tPeer.retryDelay)
				log.Printf("[ConnectionManager] Failed to reconnect to %s: %v. Next retry in %v\n", t.id, err, tPeer.retryDelay)
			} else {
				tPeer.retryDelay = 1 * time.Second
				tPeer.nextRetry = time.Time{}
			}
		}(target)
	}
}

// Port returns the dynamic TCP port of the listening socket.
func (cm *ConnectionManager) Port() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.listener == nil {
		return 0
	}
	return cm.listener.Addr().(*net.TCPAddr).Port
}
