package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"p2p/pkg/discovery"
	"p2p/pkg/ipc"
	"p2p/pkg/network"
	"p2p/pkg/protocol"
	"p2p/pkg/storage/sqlite"
	"p2p/pkg/sync"
)

const pidFilePath = "/tmp/p2p_sync.pid"

func writePIDFile() error {
	pid := os.Getpid()
	return os.WriteFile(pidFilePath, []byte(strconv.Itoa(pid)+"\n"), 0644)
}

func removePIDFile() {
	os.Remove(pidFilePath)
}

func checkAndKillStaleProcess() {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		return
	}
	oldPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || oldPID == os.Getpid() {
		return
	}
	proc, err := os.FindProcess(oldPID)
	if err != nil {
		removePIDFile()
		return
	}
	// Signal 0 checks if process exists without killing
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		removePIDFile()
		return
	}
	log.Printf("[Main] Detected stale process PID %d. Sending SIGTERM...\n", oldPID)
	proc.Signal(syscall.SIGTERM)
	// Give it a moment to release resources
	time.Sleep(500 * time.Millisecond)
	// Force kill if still alive
	if err := proc.Signal(syscall.Signal(0)); err == nil {
		log.Printf("[Main] Stale PID %d still alive. Sending SIGKILL...\n", oldPID)
		proc.Kill()
		time.Sleep(200 * time.Millisecond)
	}
	removePIDFile()
}

func probePort(port int) (bool, error) {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false, nil // port in use
	}
	ln.Close()
	return true, nil
}

func main() {
	// Before anything else: check for stale PID and release its resources
	checkAndKillStaleProcess()

	// Write PID file (remove on exit)
	if err := writePIDFile(); err != nil {
		log.Printf("[Main] Warning: Could not write PID file: %v\n", err)
	}
	defer removePIDFile()

	// Determine local peer ID (hostname)
	localPeerID, err := os.Hostname()
	if err != nil {
		localPeerID = "unknown-peer"
	}
	if envPeerID := os.Getenv("PEER_ID"); envPeerID != "" {
		localPeerID = envPeerID
	}

	// Read port config
	p2pPort := 9876
	if envPort := os.Getenv("P2P_PORT"); envPort != "" {
		if val, err := strconv.Atoi(envPort); err == nil {
			p2pPort = val
		}
	}

	// Probe and handle port conflict
	if free, err := probePort(p2pPort); err != nil || !free {
		log.Printf("[Main] Port %d is in use. Attempting recovery...\n", p2pPort)
		// Try ports sequentially as fallback
		for alt := p2pPort + 1; alt < p2pPort+100; alt++ {
			if free, _ := probePort(alt); free {
				log.Printf("[Main] Fallback: using port %d instead of %d\n", alt, p2pPort)
				p2pPort = alt
				break
			}
		}
	}

	// Read IPC socket path config
	ipcSocket := "/tmp/p2p_sync.sock"
	if envIpcSocket := os.Getenv("IPC_SOCKET"); envIpcSocket != "" {
		ipcSocket = envIpcSocket
	}

	log.Printf("[Main] PID: %d, PeerID: %s, P2P Port: %d, IPC Socket: %s\n", os.Getpid(), localPeerID, p2pPort, ipcSocket)

	// Start IPC server
	ipcServer := ipc.NewIpcServer(ipcSocket)
	if err := ipcServer.Start(); err != nil {
		log.Printf("[Main] Failed to start IPC server: %v\n", err)
		removePIDFile()
		os.Exit(1)
	}
	defer ipcServer.Stop()

	// Start peer discovery
	peerRegistry := discovery.NewPeerRegistry()
	mdnsServer, err := peerRegistry.StartDiscovery(localPeerID, p2pPort)
	if err != nil {
		log.Printf("[Main] Failed to start peer discovery: %v\n", err)
		removePIDFile()
		os.Exit(1)
	}
	defer func() {
		peerRegistry.StopDiscovery()
		if mdnsServer != nil {
			mdnsServer.Shutdown()
		}
	}()

	// Start connection manager with localPeerID
	connMgr := network.NewConnectionManager(localPeerID)
	if err := connMgr.StartServer(p2pPort); err != nil {
		log.Printf("[Main] Failed to start P2P server: %v\n", err)
		removePIDFile()
		os.Exit(1)
	}
	defer connMgr.Stop()

	// Read manual peer configurations from environment variable PEER_ADDRESSES (fallback when mDNS is blocked by router/firewall)
	if envPeers := os.Getenv("PEER_ADDRESSES"); envPeers != "" {
		log.Printf("Parsing manual PEER_ADDRESSES: %s\n", envPeers)
		for _, pStr := range strings.Split(envPeers, ",") {
			parts := strings.Split(pStr, "@")
			if len(parts) == 2 {
				peerID := parts[0]
				addrParts := strings.Split(parts[1], ":")
				if len(addrParts) == 2 {
					host := addrParts[0]
					if portVal, err := strconv.Atoi(addrParts[1]); err == nil {
						peerRegistry.AddManualPeer(peerID, host, portVal)
					}
				}
			}
		}
	}

	// Initialize sqlite DB
	dbPath := "p2p_sync.db"
	if envDbPath := os.Getenv("DB_PATH"); envDbPath != "" {
		dbPath = envDbPath
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		log.Printf("[Main] Failed to open SQLite database: %v\n", err)
		removePIDFile()
		os.Exit(1)
	}
	defer db.Close()

	// Start sync coordinator
	coord := sync.NewSyncCoordinator(db, ipcServer, connMgr, localPeerID)
	if err := coord.Start(); err != nil {
		log.Printf("[Main] Failed to start sync coordinator: %v\n", err)
		removePIDFile()
		os.Exit(1)
	}
	defer coord.Stop()

	// Hook up discovery event to trigger outward dials
	peerRegistry.OnPeerDiscovered = func(peer *discovery.Peer) {
		// Don't connect to ourselves (in case mDNS broadcasts ourselves)
		if peer.ID == localPeerID {
			return
		}

		log.Printf("Connecting to discovered peer: %s (%s:%d)\n", peer.ID, peer.Address, peer.Port)
		if err := connMgr.Connect(peer.ID, peer.Address, peer.Port); err != nil {
			log.Printf("Failed to connect to peer %s: %v\n", peer.ID, err)
		} else {
			// Trigger a peer list update back to C++ when we successfully connect
			if err := sendPeerList(peerRegistry, connMgr, ipcServer); err != nil {
				log.Printf("Failed to update peer list: %v\n", err)
			}
		}
	}

	// Hook up connection lifecycle events to notify C++
	connMgr.OnConnected = func(peerID string) {
		if err := sendPeerList(peerRegistry, connMgr, ipcServer); err != nil {
			log.Printf("Failed to update peer list: %v\n", err)
		}
	}
	connMgr.OnDisconnected = func(peerID string) {
		if err := sendPeerList(peerRegistry, connMgr, ipcServer); err != nil {
			log.Printf("Failed to update peer list: %v\n", err)
		}
	}

	// Hook up incoming P2P message forwards
	connMgr.OnMessage = func(peerID string, msg *ipc.Message) {
		log.Printf("Received P2P message from peer %s: %s\n", peerID, msg.Type)
		
		// Let the coordinator process sync-related network messages first
		if msg.Type == "file_metadata_update" || msg.Type == "file_request" || msg.Type == "file_response" {
			if err := coord.HandleP2PMessage(peerID, msg); err != nil {
				log.Printf("Coordinator failed to handle P2P message: %v\n", err)
			}
			return
		}

		// Forward any other messages to C++ daemon over IPC
		ipcServer.SendMessage(msg)
	}

	// Handle IPC messages from C++ daemon
	ipcServer.OnMessage = func(msg *ipc.Message) error {
		fmt.Printf("Received from C++: %s\n", msg.Type)

		if msg.Type == "file_changed" || msg.Type == "add_repository" || msg.Type == "remove_repository" || msg.Type == "repo_list_request" || msg.Type == "repo_status_request" {
			return coord.HandleIPCMessage(msg)
		}

		switch msg.Type {
		case "peer_list_request":
			// Send peer list back to C++
			return sendPeerList(peerRegistry, connMgr, ipcServer)
		}
		return nil
	}

	// HTTP health endpoint
	healthPort := 8080
	if envPort := os.Getenv("HEALTH_PORT"); envPort != "" {
		if val, err := strconv.Atoi(envPort); err == nil {
			healthPort = val
		}
	}
	startTime := time.Now()
	healthSrv := startHealthEndpoint(healthPort, localPeerID, p2pPort, connMgr, startTime)

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("Shutting down...")
	healthSrv.Close()
}

func handleFileChanged(msg *ipc.Message, connMgr *network.ConnectionManager) error {
	fmt.Println("Broadcasting file change to all peers...")
	connMgr.Broadcast(msg)
	return nil
}

func sendPeerList(registry *discovery.PeerRegistry, connMgr *network.ConnectionManager, ipcServer *ipc.IpcServer) error {
	peers := registry.GetPeers()
	peerList := make([]protocol.PeerInfo, 0, len(peers))

	for _, p := range peers {
		if p.ID == connMgr.LocalPeerID() {
			continue
		}
		peerList = append(peerList, protocol.PeerInfo{
			ID:        p.ID,
			Name:      p.Name,
			Address:   p.Address,
			Port:      p.Port,
			Connected: connMgr.IsConnected(p.ID),
		})
	}

	payload, err := json.Marshal(protocol.PeerListPayload{Peers: peerList})
	if err != nil {
		return fmt.Errorf("failed to marshal peer list payload: %v", err)
	}

	responseMsg := &ipc.Message{
		Version:   "1.0",
		Type:      "peer_list_update",
		ID:        fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		Source:    "go",
		Payload:   payload,
	}

	ipcServer.SendMessage(responseMsg)
	return nil
}

// HealthSummary contains the fields returned by the /health endpoint.
type HealthSummary struct {
	PID         int    `json:"pid"`
	PeerID      string `json:"peer_id"`
	P2PPort     int    `json:"p2p_port"`
	Connections int    `json:"connections"`
	Uptime      string `json:"uptime"`
	Status      string `json:"status"`
}

func startHealthEndpoint(port int, peerID string, p2pPort int, connMgr interface{ ActiveConnections() map[string]net.Conn }, startTime time.Time) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(HealthSummary{
			PID:         os.Getpid(),
			PeerID:      peerID,
			P2PPort:     p2pPort,
			Connections: len(connMgr.ActiveConnections()),
			Uptime:      time.Since(startTime).String(),
			Status:      "ok",
		})
	})
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Printf("[Main] Failed to listen on health port %d: %v\n", port, err)
		return nil
	}
	srv := &http.Server{Handler: mux}
	go func() {
		log.Printf("[Main] Health endpoint listening on %s\n", listener.Addr().String())
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[Main] Health endpoint error: %v\n", err)
		}
	}()
	return srv
}
