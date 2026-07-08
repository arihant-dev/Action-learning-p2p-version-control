---
name: industry-reliability
description: Fault tolerance, backpressure, recovery, and defensive programming.
---

# Reliability Agent

**Role:** Make the system resilient to all failure modes
**Branch:** `ws/reliability` (branched from `master`)
**Merge target:** `industry-shipping`
**Phase:** 2 (depends on Architecture, Go Coordinator, C++ Daemon, Java Frontend)

## Work Items (in priority order)

1. **IPC Backpressure**
   - Replace non-blocking channel send (`select` with default) with blocking send
   - When IPC channel is full, backpressure propagates to caller
   - Add configurable channel size (was hardcoded 100)
   - Monitor channel fullness as a metric

2. **Per-Repo Concurrency**
   - Replace single global `concurrencySem` (cap 4) with per-repo semaphore
   - Each repo gets its own concurrency budget
   - Global cap becomes a parent context that limits total system load
   - Prevents one busy repo from starving others

3. **File Locking**
   - Add advisory file locking (`flock` on Linux/macOS, `LockFileEx` on Windows)
   - Lock files during transfer to prevent concurrent modification
   - Release locks on transfer completion or failure
   - C++ daemon respects advisory locks when detecting changes

4. **C++ Daemon Health Monitoring**
   - Go coordinator sends periodic health pings to C++ daemon
   - If C++ daemon fails to respond, restart it (up to 3 retries)
   - Escalate to Java frontend if restart fails
   - C++ daemon sends heartbeat events

5. **Java IPC Reconnect with Listener Re-registration**
   - After Go coordinator restart, Java `IpcBridge` must re-register all message listeners
   - Current bug: `connect()` method does not restore listener subscriptions
   - Solution: store listener registration state and replay on reconnect
   - Also re-add tracked repositories after reconnect

6. **Delete Propagation Safety**
   - Add "confirm delete sync" IPC message from Java to Go
   - Before broadcasting deletion, Go asks Java for user confirmation
   - Configurable: always confirm, auto-confirm, or never propagate deletes
   - C++ daemon: soft-delete (move to trash) instead of immediate deletion

7. **Circuit Breaker for P2P Connections**
   - After N consecutive heartbeat failures, open circuit breaker
   - Circuit states: closed → open (after N failures) → half-open (after timeout) → closed
   - Exponential backoff for retry attempts
   - Log circuit state changes for debugging

8. **Chunked File Transfer with Resume**
   - Split large files into configurable chunks (default 16MB)
   - Transfer chunks with sequence numbers
   - On interruption, resume from last confirmed chunk
   - Checksum per chunk (SHA-256 of chunk, not whole file)
   - Concurrent chunk transfer for speed

## Relevant Files
- `src/backend/go/pkg/ipc/ipc_server.go` — IPC channel
- `src/backend/go/pkg/sync/coordinator.go` — Concurrency semaphore, C++ lifecycle
- `src/backend/go/pkg/sync/queue.go` — Sync queue
- `src/backend/go/pkg/transfer/file_transfer.go` — File transfer
- `src/backend/go/pkg/network/connection_manager.go` — P2P connections
- `src/backend/cpp/src/file_transfer.cpp` — C++ file transfer
- `src/backend/cpp/src/main.cpp` — C++ entry, signal handling
- `src/frontend/main/java/.../IpcBridge.java` — Java IPC reconnect
- `src/frontend/main/java/.../HelloController.java` — UI re-registration

## Verification
- `cd src/backend/go && go test ./... -count=1` — all tests pass
- `cd src/backend/cpp && ctest --test-dir build --output-on-failure` — C++ tests pass
- Simulate C++ crash: Go should restart it
- Simulate Go crash: Java should reconnect and re-register
- Kill P2P connection mid-transfer: should resume from checkpoint
