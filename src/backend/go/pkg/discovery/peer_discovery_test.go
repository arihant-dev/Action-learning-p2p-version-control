package discovery

import (
	"testing"
	"time"
)

func TestPeerRegistry(t *testing.T) {
	pr := NewPeerRegistry()

	discoveredChan := make(chan string, 1)
	pr.OnPeerDiscovered = func(p *Peer) {
		discoveredChan <- p.ID
	}

	mockPeer := &Peer{
		ID:       "test-peer",
		Name:     "test-peer",
		Address:  "192.168.1.10",
		Port:     9876,
		LastSeen: time.Now(),
	}

	// Manually simulate a discovery event trigger
	pr.mu.Lock()
	pr.peers[mockPeer.ID] = mockPeer
	if pr.OnPeerDiscovered != nil {
		go pr.OnPeerDiscovered(mockPeer)
	}
	pr.mu.Unlock()

	select {
	case id := <-discoveredChan:
		if id != "test-peer" {
			t.Errorf("expected discovered ID to be test-peer, got %s", id)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for discovery callback")
	}

	peers := pr.GetPeers()
	if len(peers) != 1 {
		t.Errorf("expected peers count 1, got %d", len(peers))
	}

	p := pr.GetPeer("test-peer")
	if p == nil || p.Address != "192.168.1.10" {
		t.Errorf("GetPeer failed or returned invalid peer data")
	}
}
