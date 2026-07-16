package discovery

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	// Active browse polling intervals
	browseTimeout = 8 * time.Second
	browseWait    = 3 * time.Second
	pendingTTL    = 30 * time.Second
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

	// Pending entries that arrived without resolved IPs
	pending    map[string]*pendingEntry
	pendingMu  sync.Mutex
	refreshSig chan struct{}
}

type pendingEntry struct {
	entry     *zeroconf.ServiceEntry
	firstSeen time.Time
	retries   int
}

func NewPeerRegistry() *PeerRegistry {
	return &PeerRegistry{
		peers:      make(map[string]*Peer),
		pending:    make(map[string]*pendingEntry),
		refreshSig: make(chan struct{}, 1),
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

	entries := make(chan *zeroconf.ServiceEntry, 64)
	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			pr.handlePeerDiscovered(entry)
		}
	}(entries)

	for {
		select {
		case <-ctx.Done():
			return
		case <-pr.refreshSig:
		default:
		}

		browseCtx, browseCancel := context.WithTimeout(ctx, browseTimeout)
		err := resolver.Browse(browseCtx, "_p2psync._tcp", "local.", entries)
		browseCancel()
		if err != nil && err != context.Canceled {
			log.Printf("Browse failed: %v", err)
		}

		pr.retryPendingEntries(resolver)

		select {
		case <-ctx.Done():
			return
		case <-time.After(browseWait):
		}
	}
}

func (pr *PeerRegistry) retryPendingEntries(resolver *zeroconf.Resolver) {
	pr.pendingMu.Lock()
	if len(pr.pending) == 0 {
		pr.pendingMu.Unlock()
		return
	}

	now := time.Now()
	var retryEntries []*zeroconf.ServiceEntry
	for id, p := range pr.pending {
		if now.Sub(p.firstSeen) > pendingTTL {
			delete(pr.pending, id)
			continue
		}
		if p.retries < 3 {
			p.retries++
			retryEntries = append(retryEntries, p.entry)
		}
	}
	pr.pendingMu.Unlock()

	for _, entry := range retryEntries {
		pr.resolveAndAdd(entry)
	}
}

func (pr *PeerRegistry) handlePeerDiscovered(entry *zeroconf.ServiceEntry) {
	pr.resolveAndAdd(entry)
}

func (pr *PeerRegistry) resolveAndAdd(entry *zeroconf.ServiceEntry) {
	var address string
	if len(entry.AddrIPv4) > 0 {
		address = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		address = entry.AddrIPv6[0].String()
	} else if entry.HostName != "" {
		ips, err := net.LookupIP(entry.HostName)
		if err != nil || len(ips) == 0 {
			pr.pendingMu.Lock()
			if _, exists := pr.pending[entry.Instance]; !exists {
				pr.pending[entry.Instance] = &pendingEntry{
					entry:     entry,
					firstSeen: time.Now(),
				}
			}
			pr.pendingMu.Unlock()
			return
		}
		for _, ip := range ips {
			if ip4 := ip.To4(); ip4 != nil {
				address = ip4.String()
				break
			}
		}
		if address == "" {
			address = ips[0].String()
		}
	} else {
		pr.pendingMu.Lock()
		if _, exists := pr.pending[entry.Instance]; !exists {
			pr.pending[entry.Instance] = &pendingEntry{
				entry:     entry,
				firstSeen: time.Now(),
			}
		}
		pr.pendingMu.Unlock()
		return
	}

	pr.pendingMu.Lock()
	delete(pr.pending, entry.Instance)
	pr.pendingMu.Unlock()

	peer := &Peer{
		ID:       entry.Instance,
		Name:     entry.Instance,
		Address:  address,
		Port:     entry.Port,
		LastSeen: time.Now(),
	}

	pr.mu.Lock()
	existing, exists := pr.peers[peer.ID]
	if exists {
		existing.Address = address
		existing.Port = peer.Port
		existing.LastSeen = peer.LastSeen
		pr.mu.Unlock()

		if pr.OnPeerDiscovered != nil {
			go pr.OnPeerDiscovered(existing)
		}
		log.Printf("Peer re-discovered with updated address: %s (%s:%d)\n", peer.Name, peer.Address, peer.Port)
		return
	}

	pr.peers[peer.ID] = peer
	pr.mu.Unlock()

	if pr.OnPeerDiscovered != nil {
		go pr.OnPeerDiscovered(peer)
	}
	log.Printf("Peer discovered: %s (%s:%d)\n", peer.Name, peer.Address, peer.Port)
}

// RequestRefresh signals the browse loop to perform a fresh mDNS sweep.
func (pr *PeerRegistry) RequestRefresh() {
	select {
	case pr.refreshSig <- struct{}{}:
	default:
	}
}

func (pr *PeerRegistry) AddManualPeer(id, address string, port int) {
	pr.mu.Lock()
	peer, exists := pr.peers[id]
	if !exists {
		peer = &Peer{
			ID:       id,
			Name:     id,
			Address:  address,
			Port:     port,
			LastSeen: time.Now(),
		}
		pr.peers[id] = peer
		log.Printf("Manual peer added: %s (%s:%d)\n", peer.Name, peer.Address, peer.Port)
	} else {
		peer.Address = address
		peer.Port = port
		peer.LastSeen = time.Now()
		log.Printf("Manual peer details updated: %s (%s:%d)\n", peer.Name, peer.Address, peer.Port)
	}
	pr.mu.Unlock()

	// Always trigger OnPeerDiscovered to attempt dialing since the user explicitly requested it
	if pr.OnPeerDiscovered != nil {
		go pr.OnPeerDiscovered(peer)
	}
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
