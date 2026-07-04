package discovery

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

type Peer struct {
	ID        string
	Name      string
	Address   string
	Port      int
	LastSeen  time.Time
	Connected bool
}

type PeerRegistry struct {
	peers map[string]*Peer
	mu    sync.RWMutex

	// Callbacks
	OnPeerDiscovered func(*Peer)
	OnPeerLost       func(*Peer)

	// mDNS lifecycle control
	cancelBrowse context.CancelFunc
	browseWg     sync.WaitGroup
}

func NewPeerRegistry() *PeerRegistry {
	return &PeerRegistry{
		peers: make(map[string]*Peer),
	}
}

func (pr *PeerRegistry) StartDiscovery(localPeerID string, port int) (*zeroconf.Server, error) {
	instanceName := localPeerID
	if instanceName == "" {
		instanceName, _ = os.Hostname()
	}

	if port == 0 {
		port = 9876
	}

	server, err := zeroconf.Register(instanceName, "_p2psync._tcp", "local.", port, []string{"version=1.0"}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to register service: %v", err)
	}

	// Browse for other peers with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	pr.cancelBrowse = cancel
	pr.browseWg.Add(1)
	go pr.browsePeers(ctx)

	return server, nil
}

// StopDiscovery cancels the mDNS browsing goroutine and waits for cleanup.
func (pr *PeerRegistry) StopDiscovery() {
	if pr.cancelBrowse != nil {
		pr.cancelBrowse()
		pr.cancelBrowse = nil
	}
	pr.browseWg.Wait()
}

func (pr *PeerRegistry) browsePeers(ctx context.Context) {
	defer pr.browseWg.Done()

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Printf("Failed to create mDNS resolver: %v", err)
		return
	}

	entries := make(chan *zeroconf.ServiceEntry)
	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			pr.handlePeerDiscovered(entry)
		}
	}(entries)

	// Browse until context is cancelled (zeroconf will close entries when ctx is done)
	err = resolver.Browse(ctx, "_p2psync._tcp", "local.", entries)
	if err != nil && err != context.Canceled {
		log.Printf("Browse failed: %v", err)
	}
}

func (pr *PeerRegistry) handlePeerDiscovered(entry *zeroconf.ServiceEntry) {
	var address string
	if len(entry.AddrIPv4) > 0 {
		address = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		address = entry.AddrIPv6[0].String()
	} else {
		// No IP addresses resolved yet
		return
	}

	peer := &Peer{
		ID:       entry.Instance,
		Name:     entry.Instance,
		Address:  address,
		Port:     entry.Port,
		LastSeen: time.Now(),
	}

	pr.mu.Lock()
	defer pr.mu.Unlock()

	if _, exists := pr.peers[peer.ID]; !exists {
		pr.peers[peer.ID] = peer
		if pr.OnPeerDiscovered != nil {
			go pr.OnPeerDiscovered(peer)
		}
		log.Printf("Peer discovered: %s (%s:%d)\n", peer.Name, peer.Address, peer.Port)
	}
}

func (pr *PeerRegistry) AddManualPeer(id, address string, port int) {
	peer := &Peer{
		ID:       id,
		Name:     id,
		Address:  address,
		Port:     port,
		LastSeen: time.Now(),
	}
	pr.mu.Lock()
	if _, exists := pr.peers[peer.ID]; !exists {
		pr.peers[peer.ID] = peer
		if pr.OnPeerDiscovered != nil {
			go pr.OnPeerDiscovered(peer)
		}
		log.Printf("Manual peer added: %s (%s:%d)\n", peer.Name, peer.Address, peer.Port)
	}
	pr.mu.Unlock()
}

func (pr *PeerRegistry) GetPeers() []*Peer {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	peers := make([]*Peer, 0, len(pr.peers))
	for _, p := range pr.peers {
		peers = append(peers, p)
	}
	return peers
}

func (pr *PeerRegistry) GetPeer(id string) *Peer {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	return pr.peers[id]
}
