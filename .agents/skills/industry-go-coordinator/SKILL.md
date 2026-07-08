---
name: industry-go-coordinator
description: Robust Go coordinator with structured logging, profiling, rate limiting, and persistence improvements.
---

# Go Coordinator Agent

**Role:** Harden the Go coordinator with production-grade observability, rate limiting, and data persistence
**Branch:** `ws/go-coordinator` (branched from `master`)
**Merge target:** `industry-shipping`
**Phase:** 1

## Work Items (in priority order)

1. **Structured Logging**
   - Replace all `fmt.Println` and `log.Printf` calls with zerolog or zap
   - JSON format: `{"level":"info","time":"2026-07-08T12:00:00Z","component":"coordinator","msg":"started","peer_id":"hostname"}`
   - Add component field to every log: `coordinator`, `ipc`, `network`, `discovery`, `sync`, `transfer`, `storage`
   - Add correlation IDs that flow through IPC messages for request tracing
   - Log levels: debug (development), info (normal), warn (degraded), error (failure), fatal (unrecoverable)

2. **pprof Profiling Endpoints**
   - Register pprof handlers on the HTTP server: `/debug/pprof/`, `/debug/pprof/heap`, `/debug/pprof/goroutine`, `/debug/pprof/profile`
   - These should be conditionally enabled (config flag or build tag)
   - Document how to collect a 30s CPU profile: `go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30`

3. **Rate Limiter on Broadcast()**
   - Replace unbounded goroutine spawn in `Broadcast()` with worker pool
   - Semaphore with configurable size (default: 100 concurrent broadcast tasks)
   - When pool is full, either queue (with timeout) or drop with metric increment
   - Prevent OOM/hang with 1000+ connected peers

4. **Persistent Vector Clocks**
   - Currently vector clocks exist only in memory → lost on restart
   - Add `vector_clock` column to `file_metadata` table (TEXT, JSON-serialized map)
   - After every clock mutation, persist to SQLite
   - On startup, load vector clocks from DB into memory
   - Migration: add column if not exists (ALTER TABLE IF NOT EXISTS)

5. **Small-File Inline Transfer**
   - For files < configurable size threshold (default: 1MB), transfer via base64 in P2P message
   - Current: all transfers use TCP socket handover (overhead for small files)
   - Add `file_content` field to `FileResponsePayload` message in `messages.go`
   - Go coordinator: if file size < threshold, read file, base64 encode, send in P2P message
   - C++ daemon: if `file_content` present, write directly without socket handover
   - This reduces latency for typical small files (docs, configs, code)

6. **Config File Support**
   - Read configuration from YAML/TOML file (default: `~/.config/p2p-sync/config.yaml`)
   - Config precedence: CLI flags > env vars > config file > defaults
   - Use viper for config management
   - Env var mapping: `P2P_PORT` → `p2p.port`, `PEER_ID` → `p2p.peer_id`, etc.
   - Generate default config file if not present

7. **Graceful Shutdown Improvements**
   - Drain sync queue before shutdown (process pending items up to deadline)
   - Send shutdown notification to C++ daemons before killing them
   - Wait for active file transfers to complete (with configurable timeout)
   - Save in-memory state (vector clocks, queue state) to disk before exit
   - On restart, reload saved state

8. **Connection Limits**
   - Configurable max peers (default: 100)
   - Configurable max concurrent transfers (default: 4 per peer, 16 total)
   - Reject new connections when limits reached (with descriptive IPC error)
   - Priority-based connection admission for known peers

## Relevant Files
- `src/backend/go/main.go` — Entry point, HTTP server, shutdown
- `src/backend/go/pkg/network/connection_manager.go` — Broadcast, connection management
- `src/backend/go/pkg/sync/coordinator.go` — Sync logic, vector clocks
- `src/backend/go/pkg/sync/queue.go` — Sync queue
- `src/backend/go/pkg/transfer/file_transfer.go` — File transfer
- `src/backend/go/pkg/protocol/messages.go` — Message types
- `src/backend/go/pkg/storage/sqlite/metadata.go` — Metadata store (add vector clock column)
- `src/backend/go/pkg/storage/sqlite/db.go` — Schema migrations
- `src/backend/go/go.mod` — Add dependencies (zerolog, viper, pprof)

## Verification
- `cd src/backend/go && go build ./...` — must compile
- `cd src/backend/go && go test ./... -count=1` — all tests pass
- Log output should be valid JSON
- Restart coordinator after vector clock writes: clocks should persist
- Files <1MB should transfer via inline base64 (check log for message type)
- Config file should override environment defaults
