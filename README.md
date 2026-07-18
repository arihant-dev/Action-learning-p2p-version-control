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

Download the latest release for your platform from the [Releases page](https://github.com/arihant-dev/Action-learning-p2p-version-control/releases).

| Platform | Package | How to Run |
|----------|---------|------------|
| macOS    | `.dmg`  | Open DMG, drag to Applications, launch |
| Windows  | `.msi` or `.zip` | Run the MSI Installer or extract and run `P2PVersionControl.exe` |
| Linux    | `.tar.gz` | Extract and run `P2PVersionControl/bin/P2PVersionControl` |

### Build from Source

#### Prerequisites

| Tool      | Version | Purpose              |
|-----------|---------|----------------------|
| Go        | 1.22+   | Coordinator binary   |
| JDK       | 21+     | JavaFX frontend      |
| Maven     | 3.8+    | Java build           |
| CMake     | 3.16+   | C++ daemon build     |
| g++ / MSVC| 11+     | C++ compiler         |

#### Windows

```powershell
# Build Go coordinator
cd src/backend/go
go build -o ../../../bin/p2p_sync.exe

# Build C++ daemon
cd ../cpp
cmake -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build --config Release

# Build Java frontend and package
cd ../../..
./mvnw clean package
```

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

We support multiple layers of testing, from unit tests up to cross-platform end-to-end integration and containerized network simulations:

#### 1. Unit Tests

```bash
# Go unit tests
cd src/backend/go && go test ./... -count=1

# C++ unit tests
cd src/backend/cpp && ctest --test-dir build --output-on-failure

# Java compile & checks
./mvnw clean compile
```

#### 2. Native Multi-Peer E2E Integration Tests

Runs a native local multi-peer sync network (2-peer, 3-peer mesh, conflict resolution, metrics, crash recovery, etc.) directly on your host machine. This test harness is fully compatible with Linux, macOS, and Windows:

```bash
# From project root
python3 scripts/integration_harness.py
```

*Note on Windows / Sockets:* On Windows (where Unix domain sockets are unavailable), the harness automatically runs IPC over local TCP sockets using the `IPC_TCP_PORT` configuration.
*Note on mDNS:* In environments where multicast/mDNS is blocked or restricted (like some corporate networks or CI environments), you can run with `P2P_E2E_MDNS_OPTIONAL=1` to fallback to manual routing.

#### 3. Containerized Network Partition & Mesh Tests (Docker Compose)

Runs a fully isolated network of peers, each in their own separate container (one coordinator + daemon per container). This simulates real-world conditions, including network partitions/heal scenarios, three-peer sync mesh topology, and packet routing:

```bash
# Run the Docker-based E2E harness
python3 scripts/docker_harness.py
```

This starts a clean build of your Go coordinator and C++ daemon inside a lightweight Ubuntu environment, launches a multi-peer network via Docker Compose, runs synchronization test cases, injects a partition (by separating peer containers), verifies divergence, heals the partition, and verifies convergence.

---

## CI/CD Workflows

We run a comprehensive suite of automated tests on every push/PR via GitHub Actions:
- **Build-Testing (`testing.yml`):** Runs compilers, linters, sanitizers, and unit tests across Linux, macOS, and Windows.
- **Integration Tests (`integration.yml`):** Executes native E2E integration tests across Linux, macOS, and Windows.
- **Docker E2E (`docker-e2e.yml`):** Runs the Docker-based isolated containerized peer stack (network partitions, mesh) on Linux.
- **Release Build Pipeline (`release.yml`):** Builds optimized bundles (`.msi` for Windows, `.dmg` for macOS, `.tar.gz` for Linux), generates SBOMs, and publishes releases.

## Versioning

This project follows [Semantic Versioning](https://semver.org/). Releases are built automatically from `v*` tags via GitHub Actions.

## License

This project is licensed under the [MIT License](LICENSE).

## Code of Conduct

Participation in this project is governed by our [Code of Conduct](CODE_OF_CONDUCT.md).
