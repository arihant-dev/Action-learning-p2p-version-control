package config

import (
	"os"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.PeerID == "" {
		t.Error("expected non-empty default PeerID")
	}
	if cfg.P2PPort != 9876 {
		t.Errorf("expected default P2PPort 9876, got %d", cfg.P2PPort)
	}
	if cfg.IPCSocket == "" {
		t.Error("expected non-empty default IPCSocket")
	}
	if cfg.HealthPort != 8080 {
		t.Errorf("expected default HealthPort 8080, got %d", cfg.HealthPort)
	}
	if cfg.BroadcastWorkers != 100 {
		t.Errorf("expected default BroadcastWorkers 100, got %d", cfg.BroadcastWorkers)
	}
	if cfg.MaxPeers != 100 {
		t.Errorf("expected default MaxPeers 100, got %d", cfg.MaxPeers)
	}
}

func TestLoadWithDefaults(t *testing.T) {
	os.Unsetenv("PEER_ID")
	os.Unsetenv("P2P_PORT")
	os.Unsetenv("IPC_SOCKET")

	cfg := Load()

	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.P2PPort != 9876 {
		t.Errorf("expected P2PPort 9876, got %d", cfg.P2PPort)
	}
}

func TestOverrideFromEnv(t *testing.T) {
	cfg := Defaults()

	t.Setenv("PEER_ID", "test-peer")
	t.Setenv("P2P_PORT", "12345")
	t.Setenv("IPC_SOCKET", "/tmp/test.sock")
	t.Setenv("HEALTH_PORT", "9999")

	overrideFromEnv(cfg)

	if cfg.PeerID != "test-peer" {
		t.Errorf("expected PeerID 'test-peer', got '%s'", cfg.PeerID)
	}
	if cfg.P2PPort != 12345 {
		t.Errorf("expected P2PPort 12345, got %d", cfg.P2PPort)
	}
	if cfg.IPCSocket != "/tmp/test.sock" {
		t.Errorf("expected IPCSocket '/tmp/test.sock', got '%s'", cfg.IPCSocket)
	}
	if cfg.HealthPort != 9999 {
		t.Errorf("expected HealthPort 9999, got %d", cfg.HealthPort)
	}
}

func TestOverrideFromEnvInvalidPort(t *testing.T) {
	cfg := Defaults()

	t.Setenv("P2P_PORT", "not-a-number")
	overrideFromEnv(cfg)

	if cfg.P2PPort != 9876 {
		t.Errorf("expected P2PPort to remain 9876 for invalid input, got %d", cfg.P2PPort)
	}
}
