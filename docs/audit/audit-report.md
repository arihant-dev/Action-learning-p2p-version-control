# Master Audit Report — P2P Version Control

## 1. Verdict: **USABLE WITH CAVEATS**

The application genuinely works end-to-end for its core claim: a single app launch embeds both binaries, the Java UI talks real IPC to the Go coordinator, the Go coordinator performs mDNS discovery + TCP sync + SQLite storage, and the C++ daemon uses native FSEvents + OpenSSL SHA-256 + byte-exact atomic file transfers. The test harnesses (native 7/7 + docker 3/3) verify real file content convergence. **However**, the codebase has **4 critical bugs** that cause silent data loss/divergence, **no TLS despite extensive documentation claiming it**, a broken C++ shutdown that leaks processes, and a frontend that can fail silently with zero user feedback. The app works *in ideal conditions* but will produce incorrect results in common edge cases.

## 2. What actually works today

| Capability | Evidence |
|---|---|
| mDNS peer discovery (`_p2psync._tcp`) | Live-verified; harness passes |
| Manual peer connection (`PEER_ADDRESSES` env) | Code exists, live-verified |
| TCP connection management (handshake, heartbeats ping/pong, reconnect backoff) | Code exists, live-verified |
| SQLite metadata store (WAL, FK cascade, migrations) | Code exists, live-verified |
| HTTP health endpoint on `:8080` | Live-verified |
| IPC framing (4-byte BE length + JSON) | Both Go↔C++ and Go↔Java verified matching |
| File transfer via socket-handover: Go proxies remote TCP → local ephemeral port → C++ writes → SHA-256 verify → atomic rename | Live-verified with real byte-content convergence |
| Native FSEvents watcher (macOS) — real, not polling | Verified with latency <0.5s |
| SHA-256 via OpenSSL EVP | Known-answer tests pass; matches system `shasum` |
| JavaFX UI: add/remove repos, repo list, repo status view, join repo, theme toggle, IPC to Go | All wired to real Go backend (no mocks) |
| UI spawns embedded `go_coordinator` from app bundle | Bundle layout verified; user runs one binary |
| `build_macos.sh` produces working `.app` with both binaries embedded | Run to completion, binary verified alive |
| Integration harness: 7/7 native scenarios + 3/3 docker scenarios pass | Runs on this machine |
| Cross-platform: macOS works; Linux via docker; Windows code exists | Both harnesses support it |
| C++ daemon tests: 17/17 pass, ASan/UBSan clean | Clean build with zero warnings |

## 3. What is fiction today (claimed but absent or broken)

| Claim | Reality | Source |
|---|---|---|
| mTLS 1.3 encryption with TOFU `known_peers` | `pkg/network/tls.go` has **zero callers**; all traffic plaintext; `P2P_TLS_ENABLED` only in docs | Go agent B7 |
| `.dmg` macOS releases | Pipeline only builds `.zip`; no `hdiutil` anywhere | Docs X1 |
| C++ daemon shutdown | Hangs forever on SIGINT/SIGTERM (blocked `read()`); Go masks with SIGKILL | C++ agent C1 |
| C++ daemon survives coordinator restart | No reconnect logic; events silently lost forever | C++ agent C2 |
| Conflict resolution "Keep Remote" | Does `HardDelete` + log; never fetches remote file | Go agent B6 |
| Daemon crash recovery test | Only coordinator crash tested; harness docstring admits it | Integration agent |
| Metric export (Prometheus on `:8080/metrics`) | `pkg/metrics` never registers endpoint | Go agent dead code |
| Delta sync capability in handshake | Explicitly deferred by thesis; no code exists | Docs X9 |
| 10K+ concurrent peers | Max 3 peers ever tested; marketing claim | Docs V6 |
| Peer whitelist, rate limiting | Only in security-model.md; no code path | Docs V10 |
| `scripts/rotate-cert.sh` | Does not exist | Docs V1 |
| Java tests run in CI (was compile-only) | Tests exist (24 pass) but smoke-level; no socket round-trip | Java agent |
| Windows watcher shuts down cleanly | `ReadDirectoryChangesW` re-issued while pending = API misuse/UB | C++ agent C3 |

## 4. Master Findings Table

| ID | Sev | Finding | File:Line | Component |
|---|---|---|---|---|
| M1 | **CRITICAL** | Sync tasks silently dropped, never retried — repo semaphore missing → permanent data loss | `coordinator.go:662-665` | Go/sync |
| M2 | **CRITICAL** | Metadata saved as "synced" before content fetched; failed download never retries → permanent divergence | `coordinator.go:522-546` | Go/sync |
| M3 | **CRITICAL** | Inline upload reads from CWD not repo root; can serve wrong file content; path traversal | `coordinator.go:783`, `file_transfer.go:111-116`, `messages.go:281-321` | Go/transfer |
| M4 | **CRITICAL** | Inline downloads don't verify SHA-256 against requested hash | `coordinator.go:880-898` | Go/transfer |
| M5 | **CRITICAL** | Daemon hangs forever on SIGINT (blocked `read()`); orphaned processes accumulate | `main.cpp:295`, `ipc_client.cpp:206` | C++/ipc |
| M6 | **CRITICAL** | Daemon never reconnects after coordinator restart; events silently lost | `ipc_client.cpp:229-231`, `main.cpp:174-184` | C++/ipc |
| M7 | **HIGH** | TLS claimed in 3 docs but entirely unwired in code; 200-line `tls.go` has zero callers | `pkg/network/tls.go` | Go/network |
| M8 | **HIGH** | User conflict resolution "Keep Remote" broken; "Merge" is log-only no-op | `coordinator.go:1086-1130` | Go/sync |
| M9 | **HIGH** | Windows watcher: re-issues `ReadDirectoryChangesW` while prior request pending (UB) | `readdirectorychanges_watcher.cpp:57-76` | C++/watcher |
| M10 | **HIGH** | FSEvents watcher thread leaks forever; stream_ data race → potential UAF | `fsevents_watcher.cpp:82-83,28-32,72` | C++/watcher |
| M11 | **HIGH** | UI fails silently when Go binary missing — `connect()` never throws, error path unreachable | `P2PApplication.java:71-73`, `IpcBridge.java:219-228` | Java/ipc |
| M12 | **HIGH** | IpcBridge globally hijacks System.out/err; all exceptions swallowed | `IpcBridge.java:21-32,247,436` | Java/ipc |
| M13 | **MED** | FSEvents emits absolute paths, Linux/Windows relative — IPC contract differs per platform | `fsevents_watcher.cpp:50`, `coordinator.go:320-324` | C++/watcher |
| M14 | **MED** | Whole-file read into RAM for hashing; multi-GB file = multi-GB alloc | `main.cpp:228-235` | C++/hashing |
| M15 | **MED** | Zero-byte files cannot transfer | `file_transfer.cpp:105` | C++/transfer |
| M16 | **MED** | inotify misses subdirectories created after start | `inotify_watcher.cpp:76-113` | C++/watcher |
| M17 | **MED** | Test coverage theater: 2 of 4 C++ test files never wired into CMakeLists.txt | `tests/CMakeLists.txt` | C++/tests |
| M18 | **MED** | Placebo Settings dialog — saved but never consumed by any code | `RepositoryListController.java`, `IpcBridge.java:143-145` | Java/ui |
| M19 | **MED** | Status window opens on every mouse click (no double-click guard); N clicks = N windows | `RepositoryListController.java:158-182` | Java/ui |
| M20 | **MED** | Theme toggle not persisted; desyncs from Settings | `RepositoryListController.java:128-143` | Java/ui |
| M21 | **MED** | Java has no `IPC_TCP_PORT` support; can't reach coordinator in Windows/container topologies | `IpcBridge.java:313-316` | Java/ipc |
| M22 | **MED** | Dead code: `tls.go` (entire), `pkg/config`, `pkg/metrics`, `handleFileChanged`, `RemoveTarget`, `CleanupStaleSessions`, `persistVectorClock`, `HistoryStore.UpdateStatus`, chunked transfer functions, unused protocol structs | Multiple | Go/C++ |
| M23 | **MED** | `.env` contains plaintext API key (one `git add -f` from leak) | `.env` (ignored, untracked) | Repo |
| M24 | **LOW** | `.dmg` claimed everywhere; only `.zip` ever built | `build_macos.sh`, `release.yml` | Docs/CI |
| M25 | **LOW** | C++ README wrong: binary name, C++ version, cmake version, CLI flags, BUILD_TESTS=ON needed | `src/backend/cpp/README.md` | C++/docs |
| M26 | **LOW** | Version skew: pom.xml 1.6.2 vs build scripts 1.0.1 | `pom.xml:9`, `build_macos.sh:4` | Build |
| M27 | **LOW** | 10 gofmt-unformatted files despite pre-commit config | Various | Go |
| M28 | **LOW** | IPC size mismatch: C++ accepts 10MB, Go rejects >1MB | `ipc_client.cpp:211` vs `ipc_server.go:283` | IPC |
| M29 | **LOW** | Unused ikonli deps in pom; `ConflictDialog.fxml` would crash if loaded | `pom.xml:32-40`, `ConflictDialog.fxml:8` | Java |
| M30 | **LOW** | `src/backend/cpp/build-audit/` untracked leftover pollutes `git status` | Repo root | Hygiene |

## 5. Single Most Embarrassing Finding

**The entire `pkg/network/tls.go` file (200 lines implementing `GenerateCA`, `GetTLSConfig`, mTLS 1.3 setup) has zero callers.** Every line is dead. Meanwhile `docs/security/security-model.md`, `docs/adr/001-tls-for-p2p.md`, and `docs/deploy/deployment-guide.md` describe a comprehensive mTLS architecture with TOFU, `known_peers` SQLite tables, peer whitelist, certificate rotation script, and incident response — none of which exists in code. The wire protocol in `p2p-protocol.md` even advertises `mTLS` in the handshake capabilities list. This is the largest gap between documentation and implementation in the entire project.

## 6. Ordered Fix Plan

### P0 — This Week (data loss / divergence / security holes)

| Fix | Rationale | Evidence |
|---|---|---|
| Never drop queue tasks — requeue on missing semaphore | Silent data loss in the core sync loop | `coordinator.go:662-665` (M1) |
| Don't save metadata as synced before content verified; add retry/backoff on failed downloads | Permanent divergence after any transfer failure | `coordinator.go:522-546` (M2) |
| Resolve repo-relative paths against `repo.LocalPath` in upload; reject absolute/`..` from peers; verify SHA-256 on inline download | Path traversal security hole + serving wrong content + integrity bypass | `file_transfer.go:111-116`, `coordinator.go:783,873-898`, `messages.go:281-321` (M3, M4) |
| Make IPC receive interruptible (`poll()` with timeout around `read()`) + fix reconnection | Daemon can't shut down; orphaned processes accumulate | `ipc_client.cpp:201-220,229-231`, `main.cpp:174-184` (M5, M6) |

### P1 — Next (broken features, docs fraud, silent failures)

| Fix | Rationale | Evidence |
|---|---|---|
| Wire TLS or correct docs: either connect `GetTLSConfig` into `ConnectionManager` + transfer paths behind `P2P_TLS_ENABLED`, or delete `tls.go` + fix 3 docs | Largest claim-vs-reality gap in project | `pkg/network/tls.go`, `docs/security/*`, `docs/adr/001`, `docs/deploy/*` (M7) |
| Fix conflict resolution: "Keep Remote" must fetch the file; remove log-only "Merge" | Broken documented feature | `coordinator.go:1086-1130` (M8) |
| Fix Windows watcher: don't re-issue `ReadDirectoryChangesW` while pending; use proper UTF-8↔UTF-16 conversions | API misuse + UB + non-ASCII corruption | `readdirectorychanges_watcher.cpp:57-76,40,83-84` (M9) |
| Fix FSEvents watcher lifecycle: signal the semaphore to stop thread; protect `stream_` with mutex; stop before invalidating | Thread leak + data race + potential UAF | `fsevents_watcher.cpp:82-83,28-32,72` (M10) |
| Make UI report daemon connection failures to user; unblock dead `setOnFailed` path | UI can fail invisibly forever | `P2PApplication.java:71-73`, `IpcBridge.java:219-228` (M11) |
| Route Java logging properly; stop swallowing exceptions | All stack traces vanish | `IpcBridge.java:21-32,247,436` (M12) |
| Wire Settings dialog values (ipc_socket, p2p_port, log_level) into spawned Go process | Placebo settings mislead users | `IpcBridge.java:143-145` (M18) |

### P2 — Later (polish, dead code, docs cleanup)

| Fix | Rationale |
|---|---|
| Fix `IpcServer.Stop` double-`Once` bug | `ipc_server.go:239-241` |
| Add timeout to `pendingDownloads` map to prevent leak | `coordinator.go:719-725` |
| Wire the orphan C++ test files (`test_filesystem_watcher`, `test_file_transfer`) into CMakeLists | Currently 2 of 4 test files never run |
| Switch event-path hashing to streaming `sha256_file()` | Prevents GB-level RAM allocation |
| Allow zero-byte file transfers (`expected_size < 0` check) | `file_transfer.cpp:105` |
| Add `IPC_TCP_PORT` support to Java IpcBridge | UI can't reach coordinator in Windows/container |
| Wire inotify subdirectory watch creation after startup | `inotify_watcher.cpp:76-113` |
| Delete or clean dead code: `tls.go`, `pkg/config`, `pkg/metrics`, `handleFileChanged`, `RemoveTarget`, `CleanupStaleSessions`, `persistVectorClock`, `HistoryStore.UpdateStatus`, chunked transfer functions, unused protocol structs | Thousands of lines never execute |
| Delete dead FXML (`SettingsDialog.fxml`, `ConflictDialog.fxml`) and unused ikonli deps | Dead code confusion |
| Normalize event paths in daemon: emit relative paths on all platforms | Remove reliance on Go-side path fixup |
| Align IPC max message sizes (C++ 10MB → 1MB to match Go) | `ipc_client.cpp:211` vs `ipc_server.go:283` |
| Run `gofmt -w` on 10 unformatted files | Pre-commit config demands it |
| Unify versions: pom.xml 1.6.2 || build_macos.sh 1.0.1 | Confusing |
| Delete/ignore `src/backend/cpp/build-audit/` | Pollutes `git status` |
| Rotate `.env` API key and add gitleaks pre-commit | Prevents accidental secret commit |

## 7. Doc Corrections Needed (if code kept as-is)

| Doc | What to fix |
|---|---|
| `README.md` L39-43 | Change `.dmg` to `.zip` for macOS; remove "conflict resolution" and "metrics" from harness description L158 |
| `docs/security/security-model.md` | Add prominent "NOT YET IMPLEMENTED" banner for all TLS/TOFU sections, or strike them |
| `docs/adr/001-tls-for-p2p.md` | Mark "Accepted" → "Deferred" or add implementation status |
| `docs/adr/004-native-watchers.md` | Update: native watchers ARE implemented (FSEvents/inotify/RDC) or strike the acknowledgment of polling |
| `docs/api/p2p-protocol.md` L69 | Remove `delta_sync` and `mTLS` from handshake capabilities |
| `docs/deploy/deployment-guide.md` L313-337 | Remove Prometheus metrics section or add real config path |
| `docs/user/user-manual.md` | Change Settings Font Size label (remove chat app artifact); unify DB path, socket path, backup location across the doc |
| `.github/workflows/integration.yml` | Fix branch trigger if master is now `main` |
| `src/backend/cpp/README.md` | Fix binary name (`cpp_daemon` → `p2p_daemon`), C++ standard (20), CMake (3.16), add `-DBUILD_TESTS=ON` to test instructions, fix CLI args, drop fmt and nlohmann-via-FetchContent claims |

**Most important doc fix:** wire the `.env` with a note: this file is excluded from git but contains a live API key. Add a `.env.example` without secrets.
