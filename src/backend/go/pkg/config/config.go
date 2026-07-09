package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	PeerID              string `mapstructure:"peer_id"`
	P2PPort             int    `mapstructure:"p2p_port"`
	IPCSocket           string `mapstructure:"ipc_socket"`
	DBPath              string `mapstructure:"db_path"`
	LogLevel            string `mapstructure:"log_level"`
	PprofEnabled        bool   `mapstructure:"pprof_enabled"`
	HealthPort          int    `mapstructure:"health_port"`
	CertDir             string `mapstructure:"cert_dir"`
	BroadcastWorkers    int    `mapstructure:"broadcast_workers"`
	BroadcastQueue      int    `mapstructure:"broadcast_queue"`
	MaxPeers            int    `mapstructure:"max_peers"`
	MaxTransfersPerPeer int    `mapstructure:"max_transfers_per_peer"`
	ShutdownTimeout     int    `mapstructure:"shutdown_timeout"`
}

func Defaults() *Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown-peer"
	}
	return &Config{
		PeerID:              hostname,
		P2PPort:             9876,
		IPCSocket:           "/tmp/p2p_sync.sock",
		DBPath:              "p2p_sync.db",
		LogLevel:            "info",
		PprofEnabled:        false,
		HealthPort:          8080,
		BroadcastWorkers:    100,
		BroadcastQueue:      1000,
		MaxPeers:            100,
		MaxTransfersPerPeer: 4,
		ShutdownTimeout:     30,
	}
}

func Load() *Config {
	cfg := Defaults()

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	configDir := filepath.Join(os.Getenv("HOME"), ".config", "p2p-sync")
	v.AddConfigPath(configDir)
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "Warning: error reading config: %v\n", err)
		}
	}

	v.SetEnvPrefix("P2P")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.Unmarshal(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to unmarshal config: %v\n", err)
	}

	overrideFromEnv(cfg)

	return cfg
}

func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("PEER_ID"); v != "" {
		cfg.PeerID = v
	}
	if v := os.Getenv("P2P_PORT"); v != "" {
		if val, err := strconv.Atoi(v); err == nil {
			cfg.P2PPort = val
		}
	}
	if v := os.Getenv("IPC_SOCKET"); v != "" {
		cfg.IPCSocket = v
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("HEALTH_PORT"); v != "" {
		if val, err := strconv.Atoi(v); err == nil {
			cfg.HealthPort = val
		}
	}
	if v := os.Getenv("P2P_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("P2P_PPROF_ENABLED"); v != "" {
		cfg.PprofEnabled = v == "true" || v == "1" || v == "yes"
	}
	if v := os.Getenv("P2P_BROADCAST_WORKERS"); v != "" {
		if val, err := strconv.Atoi(v); err == nil {
			cfg.BroadcastWorkers = val
		}
	}
	if v := os.Getenv("P2P_BROADCAST_QUEUE"); v != "" {
		if val, err := strconv.Atoi(v); err == nil {
			cfg.BroadcastQueue = val
		}
	}
}
