package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"p2p/pkg/discovery"
	"p2p/pkg/ipc"
	"p2p/pkg/network"
	"p2p/pkg/protocol"
	"p2p/pkg/storage/sqlite"
	"p2p/pkg/sync"
)

func main() {
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

	// Read IPC socket path config
	ipcSocket := "/tmp/p2p_sync.sock"
	if envIpcSocket := os.Getenv("IPC_SOCKET"); envIpcSocket != "" {
		ipcSocket = envIpcSocket
	}

	// Start IPC server
	ipcServer := ipc.NewIpcServer(ipcSocket)
	if err := ipcServer.Start(); err != nil {
		log.Fatalf("Failed to start IPC server: %v", err)
	}
	defer ipcServer.Stop()

	// Start peer discovery
	peerRegistry := discovery.NewPeerRegistry()
	mdnsServer, err := peerRegistry.StartDiscovery(localPeerID, p2pPort)
	if err != nil {
		log.Fatalf("Failed to start peer discovery: %v", err)
	}
	defer func() {
		if mdnsServer != nil {
			mdnsServer.Shutdown()
		}
	}()

	// Start connection manager with localPeerID
	connMgr := network.NewConnectionManager(localPeerID)
	if err := connMgr.StartServer(p2pPort); err != nil {
		log.Fatalf("Failed to start P2P server: %v", err)
	}
	defer connMgr.Stop()

	// Initialize sqlite DB
	dbPath := "p2p_sync.db"
	if envDbPath := os.Getenv("DB_PATH"); envDbPath != "" {
		dbPath = envDbPath
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	// Start sync coordinator
	coord := sync.NewSyncCoordinator(db, ipcServer, connMgr, localPeerID)
	if err := coord.Start(); err != nil {
		log.Fatalf("Failed to start sync coordinator: %v", err)
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

		if msg.Type == "file_changed" || msg.Type == "add_repository" || msg.Type == "remove_repository" {
			return coord.HandleIPCMessage(msg)
		}

		switch msg.Type {
		case "peer_list_request":
			// Send peer list back to C++
			return sendPeerList(peerRegistry, connMgr, ipcServer)
		}
		return nil
	}

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("Shutting down...")
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
