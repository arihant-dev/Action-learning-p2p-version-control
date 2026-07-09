# User Manual

P2P Version Control syncs files directly between your devices over a local network
— no central server or cloud required.

---

## Table of Contents

1. [Getting Started](#1-getting-started)
2. [Adding and Managing Repositories](#2-adding-and-managing-repositories)
3. [Understanding Sync Status](#3-understanding-sync-status)
4. [Resolving Conflicts](#4-resolving-conflicts)
5. [Managing Peers](#5-managing-peers)
6. [Settings and Configuration](#6-settings-and-configuration)
7. [Troubleshooting](#7-troubleshooting)
8. [FAQ](#8-faq)

---

## 1. Getting Started

### Download and Install

**macOS**

1. Download the latest `.dmg` from the [Releases page](https://github.com/EAinsley/Action-learning-p2p-version-control/releases)
2. Open the DMG file
3. Drag `P2PVersionControl.app` to your Applications folder
4. Launch from Applications

**Linux**

1. Download the `.tar.gz` for your architecture
2. Extract: `tar xzf P2PVersionControl-linux-x64.tar.gz`
3. Run: `./P2PVersionControl/bin/P2PVersionControl`

### First Launch

When you first launch the application:

1. **Welcome Screen** — You'll see the main window with an empty repository list
2. **Auto-Configuration** — The system generates:
   - A unique peer ID (based on your hostname)
   - TLS certificates (if enabled)
   - A SQLite database for metadata
3. **Peer Discovery** — The app starts searching for other peers on the network via mDNS

```
┌─────────────────────────────────────────────┐
│  P2P Version Control                        │
│                                             │
│  [Add Repository...]                        │
│                                             │
│  No repositories yet.                       │
│  Click "Add Repository" to start syncing.   │
│                                             │
│  Peers: 2 online                            │
│  ● Alice's MacBook                          │
│  ● Bob's Desktop                            │
└─────────────────────────────────────────────┘
```

### Setting Up Your Device

Each device runs the full stack: JavaFX UI, Go coordinator, and C++ daemon.
When you add a repository, the system starts a C++ watcher for that directory.

**To verify everything is running:**

```bash
# Check Go coordinator
pgrep -fl p2p_sync

# Check IPC socket
ls -la /tmp/p2p_sync.sock

# Check health
curl http://localhost:8080/health
```

---

## 2. Adding and Managing Repositories

### Adding a Folder to Sync

1. Click **"Add Repository"** in the main window
2. Select a folder on your computer to sync
3. Give it a name (or use the folder name as default)
4. Click **"Add"**

The app starts tracking that folder immediately:

```
┌─────────────────────────────────────────────┐
│  My Project           ● Synced, 2 peers     │
│  Documents            ● Synced, 1 peer      │
│  Photos               ○ Paused              │
│                                             │
│  [Add Repository]  [Remove]  [Pause]        │
└─────────────────────────────────────────────┘
```

### Removing a Repository

1. Select the repository you want to remove
2. Click **"Remove"**
3. Confirm deletion — this does NOT delete your files, only stops tracking

### Pausing/Resuming Sync

- Click **"Pause"** to temporarily stop syncing a repository
- Click **"Resume"** to continue

---

## 3. Understanding Sync Status

### Peer Online/Offline Indicators

```
● Alice's MacBook    Online    (green)
○ Bob's Desktop      Offline   (gray)
● Carol's Laptop    Online    (green)
```

A peer is **online** if:
- It's running the application
- TCP connection established
- Heartbeats are being exchanged (every 5 seconds)

A peer goes **offline** after 15 seconds of missed heartbeats.

### Sync Progress

When a file is being transferred, a progress bar appears:

```
┌─────────────────────────────────────────────┐
│  Syncing: design_document.pdf (24 MB)       │
│  ████████████░░░░░░░░░░  50%               │
│  From: Alice's MacBook                       │
└─────────────────────────────────────────────┘
```

### File Status Icons

| Icon | Status | Meaning |
|------|--------|---------|
| ✓    | Synced | File is up to date with all peers |
| ⟳    | Pending | File is queued for sync |
| ⚠    | Conflict | Concurrent edit detected — needs resolution |
| ✗    | Error | Sync failed (permissions, disk full) |

---

## 4. Resolving Conflicts

### What is a Conflict?

A conflict occurs when two peers edit the same file at roughly the same time,
before either change has been synced to the other.

```
Alice edits document.md (version 2)
  │
  ├── Syncs to Bob? No, Bob edits first
  │
Bob edits document.md (version 2)
  │
Both try to sync → CONFLICT
```

### Conflict Dialog

When a conflict is detected, a dialog appears:

```
┌─── Conflict Detected ──────────────────────┐
│                                             │
│  File: docs/design.md                        │
│                                             │
│  Local version (you):  Version 3            │
│  Remote version (Alice): Version 2          │
│                                             │
│  [Keep Local]  [Accept Remote]              │
│                                             │
│  Details: Both files were edited at         │
│  roughly the same time. The system needs    │
│  your guidance on which version to keep.    │
│                                             │
│  Backups are created before overwriting.    │
└─────────────────────────────────────────────┘
```

**Keep Local** — Your version is kept. The remote peer will fetch yours.

**Accept Remote** — The remote version overwrites your local file (your version is backed up as `filename.backup`).

**Merge** — Not yet implemented. For now, you must manually merge and let the next
file change propagate.

### Conflict History

You can view past conflicts in the repository status:

```
┌─── Sync History ───────────────────────────┐
│                                             │
│  10:32 AM  design.md  Conflict  Resolved   │
│  10:15 AM  notes.txt   Synced    OK        │
│  09:45 AM  readme.md   Synced    OK        │
│                                             │
└─────────────────────────────────────────────┘
```

---

## 5. Managing Peers

### Peer Discovery (mDNS)

By default, the app automatically discovers peers on the same local network using
mDNS (multicast DNS). No configuration needed for peers on the same subnet.

### Manual Peer Addition

If mDNS is blocked (e.g., on a mobile hotspot or VPN):

1. Ask your peer for their IP address and peer ID
2. Go to **Settings → Network → Add Peer Manually**
3. Enter their details: `peer_id@ip_address:port`
4. The app will attempt to connect

### Peer Status

```
┌─── Connected Peers ──────────────────────────────┐
│                                             │
│  ● Alice's MacBook                          │
│     IP: 192.168.1.100                         │
│     Latency: 2ms  Version: 1.0.0          │
│  ● Bob's Desktop                            │
│     IP: 192.168.1.101                         │
│     Latency: 5ms  Version: 1.0.0          │
│  ○ Charlie's Laptop (offline)              │
│     Last seen: 5 minutes ago               │
│                                             │
└─────────────────────────────────────────────┘
```

---

## 6. Settings and Configuration

### General Settings

| Setting | Default | Description |
|---------|---------|-------------|
| Auto-start on login | Off | Launch the app when you log in |
| Log level | Info | Control log verbosity (Debug, Info, Warn, Error) |
| Language | System | UI language |

### Network Settings

| Setting | Default | Description |
|---------|---------|-------------|
| IPC Socket Path | `/tmp/p2p_sync.sock` | Unix socket for Go↔C++ communication |
| P2P Port | 9876 | TCP port for peer-to-peer traffic |
| Health Port | 8080 | HTTP port for health checks |
| TLS Enabled | Off | Enable mutual TLS encryption |
| Peer Whitelist | (empty) | Restrict connections to specific peers |

### Appearance

| Setting | Options | Description |
|---------|---------|-------------|
| Theme | Dark, Light, System (auto) | Color scheme |
| Font Size | Small, Normal, Large | Chat message text size |

---

## 7. Troubleshooting

### Connection Issues

**Problem: Peers not discovered via mDNS**

Possible causes:
- mDNS is blocked on your network (hotspots, VPNs, corporate networks)
- Firewall blocking port 5353 (UDP)

Solutions:
- Use manual peer addition (Settings → Network → Add Peer Manually)
- Check if you can ping the peer's IP directly

**Problem: Cannot connect to a peer**

1. Verify the peer is online (check their IP from them)
2. Verify port 9876 is open on both sides
3. Check if a firewall is blocking the connection
4. Try telnet: `telnet <peer_ip> 9876`

**Problem: Connection keeps dropping**

- Check for network instability
- Reduce the heartbeat timeout: set `HEARTBEAT_TIMEOUT=30s`
- Check logs: `/tmp/p2p_go.log` for connection errors

### Sync Not Working

**Problem: Files don't appear on other peers**

1. Check the file is in a tracked repository
2. Check both peers are online
3. Check sync status: look for "Pending" or "Error" indicators
4. Check logs: `/tmp/p2p_go.log`

**Problem: Changes aren't detected**

The C++ watcher polls every 1 second for changes. If files change faster than that,
they'll be picked up in the next poll cycle.

### Performance Problems

**Problem: High CPU usage**

- The C++ watcher polls every 1s. If monitoring a large directory, reduce the scope
- Check for an infinite loop in logs: look for repeated "file_changed" for the same file

**Problem: Slow file transfers**

- Network speed is the bottleneck (see [p2p_architecture.md](../reports/architecture/p2p_architecture.md))
- Max 4 concurrent transfers
- Files are transferred smallest-first (for quick turnaround on small files)

### Collecting Logs for Support

```bash
# Collect all logs and system info
mkdir support-bundle && cd support-bundle
cp /tmp/p2p_go.log .
cp /tmp/p2p_java.log .
cp ~/.p2p/p2p_sync.db .  # (metadata, no file content)
sqlite3 p2p_sync.db ".dump"

# System info
uname -a > system-info.txt
pgrep -fl p2p > process-info.txt

# Package into archive
tar czf support-bundle.tar.gz *
```

---

## 8. FAQ

**Q: Is my data sent through the internet?**

No. P2P Version Control only works on your local network (LAN). It does not
use the internet for sync.

**Q: Can I sync a folder with files I don't want shared with anyone?**

Yes. Simply do not share the repository ID with other peers. They won't be
able to discover or sync with you without knowing your peer address and
repo ID.

**Q: What happens if two people edit the same file at the same time?**

A conflict is detected. You'll be prompted to keep your version, accept the
remote version. The non-chosen version is backed up.

**Q: How large files can I sync?**

There's no hard limit. Very large files (256MB+) are handled optimally:
- C++ uses memory-mapped hashing (faster)
- The transfer streams in 4096-byte chunks
- Max 4 concurrent transfers

**Q: Can I have multiple peers sync the same repository?**

Yes. All peers that share a repository will sync changes between each other.

**Q: Does it work on Windows?**

Windows support is currently experimental. The Go coordinator and Java frontend
work, but the C++ daemon needs Windows porting (named pipes instead of Unix sockets).

**Q: Can I use this across different networks (e.g. home and office)?**

Not directly — it's designed for LAN use. You could use a VPN to connect
the two networks, then peers would discover each other as if on the same
local network.

**Q: How do I upgrade?**

- Docker: Pull the latest image and restart
- Native: Download the latest release, install over the old version
- Build from source: Re-run the build script

**Q: What is backed up when a conflict occurs?**

The overwritten file is saved as `filename.backup` in the same directory.
It is not synced to other peers.