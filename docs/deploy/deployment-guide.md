# Deployment Guide

This guide covers deploying the P2P Version Control system in various environments:
single-node desktop, multi-node team, and Docker.

---

## Table of Contents

1. [Single-Node Desktop Deployment](#1-single-node-desktop-deployment)
2. [Multi-Node Team Deployment](#2-multi-node-team-deployment)
3. [Docker Deployment](#3-docker-deployment)
4. [Configuration Reference](#4-configuration-reference)
5. [Security Checklist](#5-security-checklist)
6. [Monitoring](#6-monitoring)
7. [Backup and Recovery](#7-backup-and-recovery)

---

## 1. Single-Node Desktop Deployment

### macOS

1. Download the `.dmg` from the [Releases page](https://github.com/arihant-dev/Action-learning-p2p-version-control/releases)
2. Open the DMG and drag `P2PVersionControl.app` to Applications
3. Launch from Applications

```bash
# Or build from source
./build_macos.sh
open target/bundle/P2PVersionControl.app
```

### Linux

```bash
# Download and extract
tar xzf P2PVersionControl-linux-x64.tar.gz
cd P2PVersionControl/bin
./P2PVersionControl
```

Or build from source:

```bash
./build_linux.sh
./target/bundle/P2PVersionControl/bin/P2PVersionControl
```

### Windows

Windows is fully natively supported. The application compiles natively and is packaged as a standard installer (`.msi`) or executable `.zip`. Because Windows does not support native Unix domain sockets on older platforms, the IPC layer automatically utilizes a secure local TCP loopback (`127.0.0.1:<IPC_TCP_PORT>`) channel for Go ↔ C++ ↔ Java coordination.

```powershell
# Run the packaged executable or installer
P2PVersionControl.exe
```

---

## 2. Multi-Node Team Deployment

### Requirements

- All nodes on the same local network (or routable with firewall rules)
- mDNS-enabled (most LANs support it; for restricted networks, use manual peer config)
- TCP port 9876 open between peers

### Setup

1. Install the application on each team member's machine
2. Configure each peer's identity via environment variables (see §4)
3. Launch the application on each machine
4. Peers auto-discover each other via mDNS (`_p2psync._tcp`)
5. For networks where mDNS is blocked (e.g. mobile hotspots), use manual peers:

```bash
P2P_ADDRESSES="bob@192.168.1.101:9876,carol@192.168.1.102:9876" ./P2PVersionControl
```

### Firewall Configuration

| Direction | Port  | Protocol | Purpose          | Rule                                        |
| --------- | ----- | -------- | ---------------- | ------------------------------------------- |
| Inbound   | 9876  | TCP      | P2P control      | Allow from trusted subnet or team VPN       |
| Inbound   | 5353  | UDP      | mDNS discovery   | Allow from local subnet (usually unrestricted) |
| Inbound   | 9999  | TCP      | IPC fallback     | Localhost only (127.0.0.1)                 |
| Inbound   | 8080  | TCP      | Health endpoint   | Localhost or internal monitoring            |

**Example iptables rules (Linux):**

```bash
# Allow P2P from team subnet
iptables -A INPUT -p tcp --dport 9876 -s 10.0.0.0/8 -j ACCEPT

# Block P2P from external
iptables -A INPUT -p tcp --dport 9876 -j DROP

# Allow health only from localhost
iptables -A INPUT -p tcp --dport 8080 -s 127.0.0.1 -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -j DROP
```

---

## 3. Docker Deployment

### Dockerfile.release

An existing `Dockerfile.linux` can be used for building inside a container.
For deployment, create a minimal runtime image:

```dockerfile
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y \
    openjdk-21-jre \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY target/bundle/P2PVersionControl /opt/p2p-sync

EXPOSE 9876 8080

VOLUME ["/data", "/config"]

ENV DB_PATH=/data/p2p_sync.db
ENV P2P_PORT=9876

CMD ["/opt/p2p-sync/bin/P2PVersionControl"]
```

### Building and Running

```bash
# Build the release
docker build -f Dockerfile.release -t p2p-sync:latest .

# Run
docker run -d \
  --name p2p-sync-node1 \
  --network host \
  -v /path/to/sync/data:/data \
  -v /path/to/config:/config \
  -e PEER_ID=node1 \
  -e P2P_TLS_ENABLED=true \
  p2p-sync:latest
```

**Note:** `--network host` is recommended for mDNS to work inside Docker.
If not using mDNS, bridge networking with exposed ports is sufficient:

```bash
docker run -d \
  --name p2p-sync-node1 \
  -p 9876:9876 \
  -p 8080:8080 \
  -e PEER_ADDRESSES="node2@10.0.0.2:9876" \
  p2p-sync:latest
```

### Docker Compose (Team Setup)

```yaml
version: '3.8'
services:
  peer1:
    image: p2p-sync:latest
    network_mode: host
    environment:
      PEER_ID: peer1
      DB_PATH: /data/p2p_sync.db
    volumes:
      - peer1-data:/data
      - peer1-sync:/sync

volumes:
  peer1-data:
  peer1-sync:
```

---

## 4. Configuration Reference

### Environment Variables

| Variable               | Default                  | Description                                         |
| ---------------------- | ------------------------ | --------------------------------------------------- |
| `PEER_ID`             | hostname                | Unique peer identifier used in handshake            |
| `P2P_PORT`            | 9876                     | TCP port for P2P connections                        |
| `IPC_SOCKET`          | `/tmp/p2p_sync.sock`    | Unix domain socket path for IPC (Linux/macOS default)|
| `IPC_TCP_PORT`        | (none)                   | Local TCP port for loopback IPC (Defaults to Unix Sockets on macOS/Linux if not set; mandatory on Windows) |
| `DB_PATH`             | `p2p_sync.db`           | Path to SQLite database file                        |
| `PEER_ADDRESSES`      | (none)                   | Manual peer list (`id@host:port,id2@host2:port2`)   |
| `P2P_E2E_MDNS_OPTIONAL`| `0`                      | If set to `1`, mDNS auto-discovery failure during testing is bypassed/skipped (useful for macOS/Windows CI machines where multicast is blocked) |
| `P2P_TLS_ENABLED`     | `false`                  | Enable mutual TLS for P2P connections               |
| `P2P_TLS_CERT_DIR`    | `~/.p2p/certs`          | Directory for TLS certificates                      |
| `P2P_CA_CERT`         | `ca.crt`                 | CA certificate filename                             |
| `P2P_PEER_CERT`       | `peer.crt`               | Peer certificate filename                           |
| `P2P_PEER_KEY`        | `peer.key`               | Peer private key filename                           |
| `P2P_PEER_WHITELIST`  | (none)                  | Comma-separated peer IDs allowed to connect         |
| `LOG_LEVEL`           | `info`                   | Log level (`debug`, `info`, `warn`, `error`)       |
| `LOG_FORMAT`          | `text`                   | Log format (`text`, `json`)                         |
| `HEARTBEAT_INTERVAL`  | `5s`                     | Time between ping messages                          |
| `HEARTBEAT_TIMEOUT`   | `15s`                    | Timeout before disconnect                           |
| `RECONNECT_BASE_DELAY`| `1s`                     | Initial reconnect delay                             |
| `RECONNECT_MAX_DELAY` | `60s`                    | Maximum reconnect delay                             |
| `HTTP_PORT`           | `8080`                   | Port for health endpoint                            |
| `MAX_TRANSFERS`       | `4`                      | Maximum concurrent file transfers                   |
| `DB_PATH`             | `p2p_sync.db`            | SQLite database file path                           |

### Configuration File Format

The system supports TOML-based config files as an alternative to environment variables:

```toml
[peer]
id = "my-laptop"
port = 9876
addresses = ["bob@192.168.1.101:9876"]

[tls]
enabled = true
cert_dir = "/home/user/.p2p/certs"

[logging]
level = "info"
format = "json"

[heartbeat]
interval = "5s"
timeout = "15s"

[reconnect]
base_delay = "1s"
max_delay = "60s"

[limits]
max_transfers = 4
```

**Precedence:** CLI flags > Environment variables > Config file > Built-in defaults

---

## 5. Security Checklist

### Pre-Deployment

- [ ] **Enable TLS:** Set `P2P_TLS_ENABLED=true`
- [ ] **Configure certificate directory:** Set `P2P_TLS_CERT_DIR` to a secure location
  (default: `~/.p2p/certs`)
- [ ] **Verify certificates:** Ensure certificates are valid and not expired
- [ ] **Set peer whitelist:** Use `P2P_PEER_WHITELIST` to restrict which peers can connect
- [ ] **Configure firewall:** Only expose port 9876 to trusted peers
- [ ] **SQLite permissions:** Ensure the database file has `0600` permissions

### Per-Machine Checklist

```bash
# 1. Verify TLS certificate permissions
ls -la ~/.p2p/certs/
# Should show: -r--------  for private key
#              -r--r--r--  for certificates

# 2. Test TLS connection
openssl s_client -connect 192.168.1.100:9876 -cert peer.crt -key peer.key

# 3. Verify whitelist
# In logs, you should see:
# "PEER_WHITELIST: alice,bob" on startup
# "PEER_WHITELIST: connection rejected from charlie" (if unapproved peer)

# 4. Firewall verification
sudo iptables -L -n | grep 9876

# 5. Check SQLite permissions
ls -la p2p_sync.db
# Expected: -rw------- 1 user staff ...
```

### Firewall Reference

| Port  | Purpose           | Should Be Exposed To     |
| ----- | ----------------- | ------------------------ |
| 9876  | P2P connections   | Trusted peers only       |
| 9999  | IPC fallback      | localhost only           |
| 8080  | Health/metrics    | localhost or monitoring  |
| 5353  | mDNS (UDP)       | Local subnet (usually open) |

---

## 6. Monitoring

### Health Endpoint

The Go coordinator exposes an HTTP health endpoint at `:8080/health`:

```bash
curl http://localhost:8080/health
```

```json
{
  "status": "ok",
  "peers_connected": 3,
  "uptime_seconds": 86400,
  "version": "1.0.0"
}
```

### Prometheus Metrics

If configured, metrics are available at `:8080/metrics`:

```
# HELP p2p_peers_connected Number of currently connected peers
# TYPE p2p_peers_connected gauge
p2p_peers_connected 3

# HELP p2p_transfers_active Currently active file transfers
# TYPE p2p_transfers_active gauge
p2p_transfers_active 2

# HELP p2p_transfers_total Total completed file transfers
# TYPE p2p_transfers_total counter
p2p_transfers_total 150

# HELP p2p_bytes_transferred_total Total bytes transferred
# TYPE p2p_bytes_transferred_total counter
p2p_bytes_transferred_total 2147483648

# HELP p2p_conflicts_detected_total Total conflicts detected
# TYPE p2p_conflicts_detected_total counter
p2p_conflicts_detected_total 3
```

### Log Aggregation

Set `LOG_FORMAT=json` for structured logging:

```json
{
  "level": "info",
  "time": "2026-07-08T10:00:00Z",
  "component": "network",
  "msg": "peer connected",
  "peer_id": "alice-laptop",
  "address": "192.168.1.100:9876"
}
```

Forward these to your log aggregator (ELK, Loki, Datadog, etc.).

---

## 7. Backup and Recovery

### SQLite Database Backup

The database stores all metadata, sync history, and peer configurations.

**Automatic backup (cron):**

```bash
# Daily backup at 2am
0 2 * * * sqlite3 /path/to/p2p_sync.db ".backup /backups/p2p_sync_$(date +\%Y\%m\%d).db"
```

**Manual backup:**

```bash
sqlite3 p2p_sync.db ".backup p2p_sync_backup.db"
```

### Certificate Backup

```bash
# Backup entire cert directory
tar czf p2p-certs-backup.tar.gz ~/.p2p/certs/

# Store securely (e.g., encrypted backup)
gpg --encrypt --recipient your-key p2p-certs-backup.tar.gz
```

### Restore Procedure

**To restore from backup:**

1. Stop the application
2. Restore the database:
   ```bash
   cp /backups/p2p_sync_20260707.db p2p_sync.db
   ```
3. Restore certificates (if needed):
   ```bash
   tar xzf p2p-certs-backup.tar.gz -C ~/.p2p/
   ```
4. Restart the application
5. Verify peer connections and sync status

### Recovery Scenarios

| Scenario | Impact | Recovery Action |
|----------|--------|-----------------|
| Lost database | Lose sync history | Restore from backup, peers will re-sync files |
| Corrupted DB | Partial data loss | Run `sqlite3 p2p_sync.db "PRAGMA integrity_check;"`, restore from backup if needed |
| Expired certs | No new connections | Regenerate certificates (see Security Model doc) |
| Stale PID file | Won't start | `rm /tmp/p2p_sync.pid` and restart