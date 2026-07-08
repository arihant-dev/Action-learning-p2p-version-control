---
name: industry-architecture
description: Production-ready architecture, configuration, observability, and deployment infrastructure.
---

# Architecture & Infrastructure Agent

**Role:** Build production-grade configuration, logging, metrics, and deployment infrastructure
**Branch:** `ws/architecture` (branched from `master`)
**Merge target:** `industry-shipping`
**Phase:** 1

## Work Items (in priority order)

1. **Configuration System**
   - Add TOML/YAML config file support (use viper or similar)
   - Config precedence: CLI flags > env vars > config file > defaults
   - Configurable: socket paths, ports, DB path, log level, temp directories
   - Convert hardcoded paths in `main.go`, `IpcBridge.java`, `ipc_client.cpp` to config-driven

2. **Structured Logging (Go)**
   - Replace `fmt.Println` / `log.Printf` with zerolog or zap
   - JSON output format with timestamps, levels, correlation IDs
   - Log levels: debug, info, warn, error, fatal
   - Add request-scoped correlation IDs that flow through IPC

3. **Metrics Endpoint (Go)**
   - Add Prometheus `/metrics` HTTP endpoint alongside `/health`
   - Metrics to expose: connected peers, transfers active, queue depth, IPC messages processed, error counters
   - Use `prometheus/client_golang` or manual expvar

4. **OpenTelemetry Tracing**
   - Add OpenTelemetry instrumentation for key operations:
     - File sync end-to-end latency
     - IPC message processing
     - P2P message handling
     - File transfer duration
   - Export to OTLP-compatible backends (configurable)

5. **Graceful Degradation**
   - Go coordinator: auto-restart C++ daemon if it crashes
   - Monitor C++ daemon health via IPC heartbeat
   - Java IpcBridge: detect Go coordinator restart and re-register listeners

6. **Configurable Temp Paths**
   - Replace all `/tmp/p2p_*` hardcoded paths with configurable locations
   - Fall back to OS temp dir if not configured

## Relevant Files
- `src/backend/go/main.go` — Entry point, currently hardcoded paths
- `src/backend/go/pkg/ipc/ipc_server.go` — IPC server
- `src/backend/go/pkg/sync/coordinator.go` — C++ daemon lifecycle
- `src/frontend/main/java/.../IpcBridge.java` — Java-side Go process management
- `src/backend/go/go.mod` — Add dependencies

## Verification
- `cd src/backend/go && go build ./...` — must compile
- `cd src/backend/go && go test ./... -count=1` — all tests pass
- Verify `/metrics` endpoint returns valid Prometheus format
