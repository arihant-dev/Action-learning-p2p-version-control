# Developer Onboarding Guide

Welcome to the P2P Version Control project! This guide will get you from zero to productive
in under an hour.

---

## Repository Structure

```
.
├── build_macos.sh              # macOS application bundle build
├── build_linux.sh              # Native Linux build script
├── build_linux_docker.sh       # Docker-based Linux build
├── Dockerfile.linux            # Linux build container image
├── pom.xml                     # Maven project (JavaFX frontend, JDK 21)
├── .github/
│   └── workflows/
│       ├── testing.yml         # CI: Go tests + C++ tests on push
│       └── release.yml         # Release build pipeline
├── src/
│   ├── backend/
│   │   ├── go/                 # Go coordinator
│   │   │   ├── main.go         # Entry point, signal handling, wiring
│   │   │   ├── go.mod          # Go module (1.22)
│   │   │   └── pkg/
│   │   │       ├── discovery/  # mDNS peer discovery (zeroconf)
│   │   │       ├── ipc/        # IPC server (Unix socket, framed JSON)
│   │   │       ├── network/    # TCP connection manager, handshake, heartbeat
│   │   │       ├── protocol/   # Message type definitions and validation
│   │   │       ├── storage/    # SQLite database (metadata, history)
│   │   │       ├── sync/      # Sync coordinator, priority queue
│   │   │       ├── transfer/    # File transfer proxy (Go as middleman)
│   │   │       └── versioning/ # Lamport clock, vector clock, conflict detection
│   │   └── cpp/                # C++ watcher daemon
│   │       ├── CMakeLists.txt # CMake build (C++20)
│   │       ├── include/         # Public headers
│   │       └── src/             # Sources
│   │           ├── main.cpp                 # Entry, IPC client setup
│   │           ├── ipc_client.cpp           # Unix socket IPC client
│   │           ├── filesystem_watcher.cpp        # Polling-based watcher
│   │           ├── sha256.cpp                   # SHA-256 hashing
│   │           └── file_transfer.cpp          # Socket file upload/download
│   └── frontend/
│       ├── main/
│       │   ├── java/org/codehaus/mojo/frontendtest/
│       │   │   ├── Launcher.java             # Entry point
│       │   │   ├── HelloApplication.java   # JavaFX lifecycle
│       │   │   ├── IpcBridge.java           # IPC client + Go process mgmt
│       │   │   ├── HelloController.java     # Main controller
│       │   │   ├── RepositoryListController.java
│       │   │   └── RepoStatusController.java
│       │   └── resources/                  # FXML layouts, CSS themes
│       └── test/java/                       # JUnit 5 tests (pending)
└── docs/                                   # Documentation (this guide lives here)
    ├── adr/                                # Architecture Decision Records
    ├── api/                                # IPC & P2P protocol specs
    ├── guide/                              # Developer & user guides
    ├── deploy/                             # Deployment guides
    ├── security/                           # Security model
    └── user/                                # User manual
```

---

## Prerequisites

| Tool      | Minimum Version | Purpose                | Verification         |
| --------- | --------------- | ---------------------- | -------------------- |
| Go        | 1.22            | Coordinator binary     | `go version`         |
| JDK       | 21+             | JavaFX frontend        | `javac --version`    |
| Maven     | 3.8+            | Java build             | `mvn --version`      |
| CMake     | 3.16+           | C++ daemon build       | `cmake --version`    |
| g++/clang | 11+ / 14+       | C++ compiler           | `g++ --version`      |

### Installing Prerequisites

**macOS:**
```bash
brew install go maven cmake
# JDK 21: https://adoptium.net/
```

**Ubuntu/Debian:**
```bash
sudo apt install golang-go maven cmake g++ openjdk-21-jdk
```

**Arch Linux:**
```bash
sudo pacman -S go jdk21-openjdk maven cmake gcc
```

---

## Quick Start

### 1. Clone the Repository

```bash
git clone <repository-url>
cd p2p-version-control
```

### 2. Build the Go Coordinator

```bash
cd src/backend/go
go build ./...
```

### 3. Build the C++ Daemon

```bash
cd src/backend/cpp
cmake -B build
cmake --build build
```

### 4. Compile the Java Frontend

```bash
# From project root
./mvnw compile
```

### 5. Full Application Build

**macOS:**
```bash
./build_macos.sh
open target/bundle/P2PVersionControl.app
```

**Linux:**
```bash
./build_linux.sh
./target/bundle/P2PVersionControl/bin/P2PVersionControl
```

---

## Development Workflow

### Branch Naming

| Prefix | Purpose |
|--------|---------|
| `ws/<name>` | Workstream (long-lived feature branches) |
| `feature/<name>` | Individual feature |
| `fix/<name>` | Bug fix |
| `docs/<name>` | Documentation changes |
| `test/<name>` | Test additions or fixes |

### Commit Messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>: <short description>

[optional body]

[optional footer]
```

**Types:** `feat`, `fix`, `docs`, `test`, `refactor`, `ci`, `chore`, `perf`

Examples:
```
feat: add delta sync for large file optimization
fix: prevent panic on nil IPC connection
docs: add conflict resolution flow diagram
```

### Running Tests

```bash
# Go tests (all packages)
cd src/backend/go && go test ./... -count=1 -v

# C++ tests
cd src/backend/cpp && ctest --test-dir build --output-on-failure

# Java compilation check
./mvnw compile

# End-to-end integration test
python3 .agents/skills/p2p-multi-agent-testing/scripts/integration_harness.py
```

### Pre-commit Hooks

If `.pre-commit-config.yaml` exists at the project root:

```bash
pip install pre-commit
pre-commit install
```

This runs `gofmt`, `clang-format`, and basic lint checks before each commit.

---

## How to Add a New IPC Message Type

IPC flows between Java ↔ Go ↔ C++ over a Unix socket using framed JSON.

**Step 1:** Add the message type name to `src/backend/go/pkg/protocol/messages.go`:

```go
const (
    MsgFileChanged          = "file_changed"
    MsgAddRepository        = "add_repository"
    MsgYourNewMessage       = "your_new_message"  // ← add here
)
```

**Step 2:** Add handling logic in Go (e.g. in `main.go` or `ipc_server.go`):

```go
case "your_new_message":
    handleYourNewMessage(msg)
```

**Step 3:** Update the Java IPC sender in `IpcBridge.java`:

```java
public void sendYourNewMessage(String data) {
    JsonObject payload = new JsonObject();
    payload.addProperty("data", data);
    sendMessage("your_new_message", payload);
}
```

**Step 4:** Update the C++ IPC client handler in `main.cpp`:

```cpp
if (type == "your_new_message") {
    handleYourNewMessage(payload);
}
```

**Step 5:** Update the IPC protocol spec at `docs/api/ipc-protocol.yaml` (add payload schema).

---

## How to Add a New P2P Message Type

P2P messages flow between Go coordinators of different peers over TCP.

**Step 1:** Add the message type to `src/backend/go/pkg/network/connection_manager.go`
or a dedicated handler file:

```go
const P2PMsgTypeYourNewMessage = "your_new_p2p_message"
```

**Step 2:** Register a handler in the connection manager's message routing:

```go
func (cm *ConnectionManager) handleMessage(peerID string, msg *Message) {
    switch msg.Type {
    case P2PMsgTypeYourNewMessage:
        cm.handleYourNewMessage(peerID, msg)
    }
}
```

**Step 3:** Send the message:

```go
msg := &Message{
    Version: "1.0",
    Type:    P2PMsgTypeYourNewMessage,
    Payload: marshalPayload(...),
}
conn.Write(msg) // 4-byte length prefix + JSON
```

**Step 4:** Update the P2P protocol spec at `docs/api/p2p-protocol.md`.

---

## Debugging Tips

### Log Files

| Log          | Location              | Source |
|--------------|-----------------------|-------|
| Go           | `/tmp/p2p_go.log`   | Go coordinator |
| Java         | `/tmp/p2p_java.log`   | JavaFX frontend |
| C++ (stdout) | Captured by Go coordinator | C++ daemon |
| SQLite       | `p2p_sync.db`         | Metadata database |

### IPC Socket

```bash
# Verify IPC socket exists
ls -la /tmp/p2p_sync.sock

# Monitor IPC traffic (requires jq)
sudo tcpdump -i lo0 -A port 9999 | grep "type"  # TCP fallback

# Watch the socket file descriptor
lsof /tmp/p2p_sync.sock
```

### P2P Traffic

```bash
# Monitor P2P traffic on port 9876
sudo tcpdump -i en0 port 9876 -X

# With JSON decoding
sudo tcpdump -i en0 port 9876 -A | grep -E '^[0-9]' | jq '.type' 2>/dev/null
```

### Profiling (pprof)

The Go coordinator exposes pprof endpoints on port 8080:

```bash
# Heap profile
go tool pprof http://localhost:8080/debug/pprof/heap

# CPU profile
go tool pprof http://localhost:8080/debug/pprof/profile

# Goroutine dump
go tool pprof http://localhost:8080/debug/pprof/goroutine
```

### Health Check

```bash
# Quick health check
curl http://localhost:8080/health
# Expected: {"status":"ok"}
```

### Check Process Status

```bash
# Are the processes running?
pgrep -fl p2p_sync   # Go coordinator
pgrep -fl cpp_daemon # C++ daemon(s)

# Check PID file
cat /tmp/p2p_sync.pid
```

---

## Architecture Overview

### Three-Language Polyglot System

```
┌──────────────────────────────────────────────────────────────┐
│                   JavaFX Frontend (Java 21)                    │
│      Repository List / Repo Status / Theme Toggle            │
│                   IPC (Unix socket / TCP)                    │
├──────────────────────────────────────────────────────────────┤
│                    Go Coordinator (Go 1.22)                   │
│  • Peer discovery (mDNS + manual)                           │
│  • Connection management  (TCP + heartbeats)                │
│  • Lamport/Vector clocks for causal ordering                │
│  • File transfer orchestration (socket handover)            │
│  • Metadata storage (SQLite)                                │
│  • HTTP health endpoint (:8080)                             │
│                  IPC (Unix socket)        P2P  (TCP)        │
├──────────────────────┬────────────────────┬──────────────────┤
│  C++ Daemon (C++20)   │              Remote Peers             │
│  • File watcher       │              • Discovery / sync       │
│  • SHA-256 hashing    │                                       │
│  • File transfer      │                                       │
└──────────────────────┘                                       ┘
```

### Key Design Principles

| Principle | Implementation |
|-----------|---------------|
| **Go manages "what and when"** | Sync coordination, conflict resolution, peer management |
| **C++ handles "how"** | Disk I/O, hashing, file watching, atomic operations |
| **Java presents the UI** | User-facing repository management and status views |
| **mDNS for discovery** | Automatic peer discovery on LAN |
| **Control/data plane split** | JSON for metadata, raw TCP for file content |

### Data Flow

```
Local change → C++ watcher detects → IPC to Go
  → Go broadcasts metadata to peers → Conflict detection
  → File request/response → Go proxy TCP stream
  → C++ writes to disk → Atomic rename → Verify hash
```