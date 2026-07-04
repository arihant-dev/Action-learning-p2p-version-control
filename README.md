# P2P Version Control

A peer-to-peer version control system that syncs files directly between devices over a local network — no central server required.

Built with a **Go coordinator**, a **C++ filesystem watcher**, and a **JavaFX desktop UI**.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                 JavaFX Frontend                  │
│  (Repository List / Repo Status / Theme Toggle) │
└──────────────────────┬──────────────────────────┘
                       │ IPC (Unix Socket / TCP)
┌──────────────────────▼──────────────────────────┐
│              Go Coordinator (sync)               │
│  • Peer discovery (mDNS + manual)               │
│  • Connection management (TCP + heartbeats)     │
│  • Lamport/Vector clocks for causal ordering    │
│  • File transfer orchestration (socket handover)│
│  • Metadata storage (SQLite)                    │
│  • HTTP health endpoint (:8080)                 │
└──────┬──────────────────────────────┬───────────┘
       │ IPC (Unix Socket)            │ P2P (TCP)
┌──────▼───────────┐    ┌────────────▼──────────┐
│  C++ Daemon       │    │  Remote Peers          │
│ • File watcher    │    │  (discovery / sync)    │
│ • SHA-256 hashing │    └───────────────────────┘
│ • File transfer   │
└──────────────────┘
```

## Quick Start

### Download

Download the latest release for your platform from the [Releases page](https://github.com/EAinsley/Action-learning-p2p-version-control/releases).

| Platform | Package | How to Run |
|----------|---------|------------|
| macOS    | `.dmg`  | Open DMG, drag to Applications, launch |
| Linux    | `.tar.gz` | Extract and run `P2PVersionControl/bin/P2PVersionControl` |

### Build from Source

#### Prerequisites

| Tool      | Version | Purpose              |
|-----------|---------|----------------------|
| Go        | 1.22+   | Coordinator binary   |
| JDK       | 21+     | JavaFX frontend      |
| Maven     | 3.8+    | Java build           |
| CMake     | 3.16+   | C++ daemon build     |
| g++       | 11+     | C++ compiler         |

#### macOS

```bash
./build_macos.sh
open target/bundle/P2PVersionControl.app
```

#### Linux

```bash
./build_linux.sh
./target/bundle/P2PVersionControl/bin/P2PVersionControl
```

#### Linux (Docker)

```bash
./build_linux_docker.sh
# Output: target/P2PVersionControl-linux-x64.tar.gz
```

## Usage

1. **Launch the app** — the first screen shows your tracked repositories.
2. **Add a repository** — track a local folder to sync.
3. **Share with peers** — other instances on the same network will discover you via mDNS.
4. **Join a repository** — enter a peer's repo ID to start syncing.
5. **Theme toggle** — switch between dark and light mode.

## Development

### Project Structure

```
├── build_macos.sh              # macOS build script
├── build_linux.sh              # Linux build script
├── build_linux_docker.sh       # Docker-based Linux build
├── Dockerfile.linux            # Docker image for Linux builds
├── pom.xml                     # Maven project (JavaFX frontend)
├── src/
│   ├── backend/
│   │   ├── go/                 # Go coordinator
│   │   │   ├── main.go
│   │   │   └── pkg/
│   │   │       ├── discovery/  # mDNS peer discovery
│   │   │       ├── ipc/        # IPC server (Go ↔ C++)
│   │   │       ├── network/    # TCP connection manager
│   │   │       ├── protocol/   # Message types
│   │   │       ├── storage/    # SQLite metadata store
│   │   │       ├── sync/       # Sync coordinator + queues
│   │   │       ├── transfer/   # File transfer manager
│   │   │       └── versioning/ # LWW + vector clocks
│   │   └── cpp/                # C++ daemon
│   │       ├── include/        # Headers
│   │       └── src/            # Sources
│   └── frontend/
│       ├── main/
│       │   ├── java/           # JavaFX controllers
│       │   └── resources/      # FXML layouts, CSS themes
│       └── test/               # Java tests
└── .github/workflows/
    ├── testing.yml             # CI (unit + integration)
    └── release.yml             # Release build pipeline
```

### Running Tests

```bash
# Go tests (all packages)
cd src/backend/go && go test ./... -count=1

# C++ tests
cd src/backend/cpp && ctest --test-dir build --output-on-failure

# Java compilation check
./mvnw compile
```

## Versioning

This project follows [Semantic Versioning](https://semver.org/). Releases are built automatically from `v*` tags via GitHub Actions.

## License

MIT
