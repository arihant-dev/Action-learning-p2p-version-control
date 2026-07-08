---
name: industry-testing
description: Comprehensive testing — property-based, fuzz, integration, stress, and mutation testing.
---

# Testing Agent

**Role:** Build comprehensive test coverage across all layers of the system
**Branch:** `ws/testing` (branched from `master`)
**Merge target:** `industry-shipping`
**Phase:** 1

## Work Items (in priority order)

1. **Go Property-Based Testing**
   - Use `pgregory.net/rapid` for property-based tests
   - `versioning`: random VectorClock sequences → always converge; ConflictDetector is deterministic
   - `sync/queue`: random task insertions → fair round-robin property holds
   - `network`: random message sequences → no deadlock/crash
   - `protocol`: random payload generation → marshal/unmarshal roundtrip is idempotent

2. **Go Fuzz Testing**
   - `protocol/messages`: fuzz JSON parsing — malformed input should not crash
   - `ipc/ipc_server`: fuzz framed message parsing — length prefix + payload
   - `network/connection_manager`: fuzz handshake sequences
   - Use Go standard `testing/fuzz` (available in Go 1.22)
   - Create `fuzz_test.go` files in each package

3. **Go Integration Tests — Network Partitions**
   - Add test for: peer disconnects during sync → reconnects → resumes
   - Add test for: 3+ peers with concurrent edits → all converge to same state
   - Add test for: peer with stale clock reconnects → clocks reconcile
   - Add test for: file transfer interrupted → partial transfer cleaned up
   - Add test for: C++ daemon crashes → Go detects and restarts

4. **Go Integration Tests — Large Files & Stress**
   - Stress test: 10,000 small files (1KB each) synced between 2 peers
   - Stress test: 10 large files (100MB each) transferred concurrently
   - Stress test: rapid file changes (100 changes per second) → no event loss
   - Stress test: 50 concurrent peers → connection manager stability

5. **C++ Unit Tests (GoogleTest)**
   - `ipc_client_test`: connect/disconnect, send/receive, malformed messages, timeout
   - `filesystem_watcher_test`: file create/modify/delete detection, debounce, recursive watch
   - `sha256_test`: known vectors, empty input, large input, streaming
   - `file_transfer_test`: download/upload success, hash mismatch, atomic rename, permission apply
   - Integrate with CTest in CMakeLists.txt
   - Code coverage target: >70%

6. **Java JUnit 5 Tests**
   - `IpcBridgeTest`:
     - Connect/disconnect
     - Send message → receive response
     - Go process crash → reconnect → listener re-registration
     - Malformed IPC response handling
   - `RepositoryListControllerTest`:
     - Load repo list → display
     - Add repo → new item appears
     - Remove repo → item disappears
   - `RepoStatusControllerTest`:
     - Display status for empty repo
     - Conflict received → show conflict UI
     - Resolution action → sends IPC message
   - `HelloControllerTest`:
     - Navigation between views
     - Theme toggle
   - Use TestFX for JavaFX component testing (add to pom.xml)

7. **E2E Python Harness Enhancements**
   - Extend `.agents/skills/p2p-multi-agent-testing/scripts/integration_harness.py`:
     - Support 3+ peers
     - Concurrent file edits on multiple peers
     - Network partition simulation (block port)
     - C++ daemon crash recovery
     - Large file transfer
     - Conflict scenario → verify resolution

8. **Mutation Testing (Go)**
   - Use `github.com/zimmski/go-mutesting` or similar
   - Mutate conflict resolution logic → tests should catch incorrect outcomes
   - Mutate queue scheduling → tests should detect unfair ordering
   - Target mutation score >80%

## Relevant Files
- `src/backend/go/pkg/*/*_test.go` — Existing Go tests (add to these)
- `src/backend/cpp/tests/` — C++ test directory
- `src/frontend/main/test/` — Java test directory (may not exist yet)
- `.agents/skills/p2p-multi-agent-testing/scripts/integration_harness.py` — Python E2E harness
- `pom.xml` — Add TestFX, Mockito dependencies
- `src/backend/cpp/CMakeLists.txt` — Add GoogleTest

## Verification
- `cd src/backend/go && go test ./... -count=1 -fuzz=. -fuzztime=10s` — fuzz tests pass
- `cd src/backend/go && go test ./... -count=1` — all unit and integration tests pass
- `cd src/backend/cpp && ctest --test-dir build --output-on-failure` — C++ tests pass
- `./mvnw test` — Java tests pass
- `python3 .agents/skills/p2p-multi-agent-testing/scripts/integration_harness.py` — E2E passes
- Mutation score >80% for key packages
