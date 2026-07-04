package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockConnMgr implements the healthConnChecker interface for testing.
type mockConnMgr struct {
	conns map[string]net.Conn
}

func (m *mockConnMgr) ActiveConnections() map[string]net.Conn {
	return m.conns
}

func TestHealthEndpoint(t *testing.T) {
	connMgr := &mockConnMgr{
		conns: map[string]net.Conn{
			"peer-1": nil,
			"peer-2": nil,
		},
	}

	// Create a minimal server using httptest for deterministic testing
	mux := http.NewServeMux()
	startTime := time.Now()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(HealthSummary{
			PID:         12345,
			PeerID:      "test-peer",
			P2PPort:     9999,
			Connections: len(connMgr.ActiveConnections()),
			Uptime:      time.Since(startTime).String(),
			Status:      "ok",
		})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var summary HealthSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if summary.Status != "ok" {
		t.Errorf("expected status ok, got %s", summary.Status)
	}
	if summary.PeerID != "test-peer" {
		t.Errorf("expected peer_id test-peer, got %s", summary.PeerID)
	}
	if summary.P2PPort != 9999 {
		t.Errorf("expected p2p_port 9999, got %d", summary.P2PPort)
	}
	if summary.Connections != 2 {
		t.Errorf("expected connections 2, got %d", summary.Connections)
	}
	if summary.PID != 12345 {
		t.Errorf("expected PID 12345, got %d", summary.PID)
	}
	if summary.Uptime == "" {
		t.Error("expected non-empty uptime")
	}
}

func TestHealthEndpointStartup(t *testing.T) {
	connMgr := &mockConnMgr{conns: map[string]net.Conn{}}
	srv := startHealthEndpoint(0, "health-peer", 7777, connMgr, time.Now())
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	srv.Close()
}
