# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [1.6.0] - 2026-07-16

### Added
- Active mDNS peer discovery with periodic re-browse, HostName fallback, and pending-entry retry.
- `peer_list_update` / `repo_list_response` replay buffer for late-connecting IPC clients.
- Per-file version counter (files now start at v1 and only increment on actual changes).
- Lamport clock restoration from the database on startup.
- Cross-platform path normalization to forward slashes across C++ daemon and Go coordinator.
- Peer count indicator in the main repository list view.

### Fixed
- Peers not appearing on first launch; required closing and reopening the app.
- Version numbers inflating globally (e.g., showing v56 on first sync) due to shared Lamport clock.
- `Witness()` was called unconditionally on every peer metadata update, causing version inflation.
- Stale peer target addresses persisted after re-discovery.
- macOS ↔ Windows sync failures caused by backslash path separators and mismatched permissions.
- Linux inotify watcher dropping subdirectory prefixes from file paths.
- Duplicate `WindowsIpcClient` class causing ODR link errors on Windows.
- C++ IPC partial read/write bugs on both Unix and Windows.

## [1.5.0] - 2026-07-13

### Added
- Real end-to-end integration assertions replacing prior no-op scaffolds: conflict detection (via the SQLite `sync_history` table), network-partition heal, and coordinator crash recovery; plus a dedicated mDNS auto-discovery test. Deterministic peer wiring keeps the suite stable under CI load.
- CI hardening: Go tests run with the race detector + coverage, `go vet`, and bounded fuzz smoke; C++ unit tests are now actually built in CI (`-DBUILD_TESTS=ON`) with an added AddressSanitizer/UBSan job; Java tests now run in CI (previously compile-only).
- Release signing pipeline: conditional macOS code-signing + notarization + stapling, Windows Authenticode MSI signing, and `SHA256SUMS` with an optional GPG signature; unsigned-but-checksummed artifacts are produced when signing secrets are absent. See `docs/deploy/releasing.md`.
- `LICENSE` (MIT) and `CODE_OF_CONDUCT.md` (Contributor Covenant).
- `P2P_DISABLE_MDNS` environment flag for deterministic, manually-wired peer topologies.

### Changed
- Rebranded the desktop app from the Maven-archetype leftover `org.codehaus.mojo.frontendtest` to `io.p2pvcs.app` (entry class `P2PApplication`, Maven coordinates `io.p2pvcs:p2p-version-control`).

### Fixed
- Data race on `ConnectionManager` callbacks (`OnConnected`/`OnDisconnected`/`OnMessage`) behind the flaky `TestAutoReconnect`; callback access is now synchronized and the full Go suite passes under `-race`.
- `ReadMessage` accepted IPC messages with an empty `type` (found by fuzzing); such messages are now rejected, with a committed regression corpus.
- Removed a `go vet` lock-copy warning (`TransferSession` copied by value in a stress test).

## [1.0.1] - 2026-07-07

### Added
- Multi-architecture matrix builds in the GitHub Actions release workflow to produce native Intel (`x64`) and Apple Silicon (`arm64`) macOS app bundles.
- Dynamic target architecture name suffix (`x64`/`arm64`) appended to generated macOS zip archives.
- Manual Peer Connection: Added UI button and dialog in `RepoStatusView` to connect to peers directly via IP address, bypassing mDNS multicast blocking on restricted networks (like mobile hotspots).

### Fixed
- Fixed macOS app packaging failures in `build_macos.sh` caused by iCloud/FileProvider metadata and dynamic `com.apple.FinderInfo` extended attributes.
- Resolved database and UNIX socket initialization crashes when running the packaged `.app` bundle from Finder or under macOS App Translocation.

## [1.0.0] - 2026-07-04

### Added
- Peer-to-peer file synchronization over local network
- mDNS peer discovery with manual peer fallback
- TCP connection management with heartbeat-based failure detection
- Automatic reconnection with exponential backoff
- Lamport and Vector clocks for causal ordering of edits
- LWW (Last-Writer-Wins) conflict resolution
- SQLite-based metadata storage
- File transfer with socket handover to C++ daemon
- SHA-256 content hashing for change detection
- JavaFX desktop UI with repository list and status views
- Dark/light theme toggle
- IPC bridge between Java frontend, Go coordinator, and C++ watcher
- C++ filesystem watcher with polling-based change detection
- Health endpoint (`GET /health` on `:8080`)
- Support for adding, joining, sharing, and deleting repositories
- Cross-platform build scripts for macOS (native) and Linux (native + Docker)
- GitHub Actions CI with Go, C++, and Java tests on Ubuntu + macOS

### Fixed
- Go coordinator: send-on-closed-channel panic during shutdown
- Go coordinator: mutual-connection race destroying valid connections
- Go coordinator: mDNS goroutine leak on shutdown
- Go coordinator: unchecked `json.Marshal` errors (10 instances)
- Go coordinator: unbounded session map growth (now purges at 1000 entries)
- Go coordinator: missing payload validation in message handlers
- C++ daemon: infinite loop in file transfer when `expected_size` is invalid
- C++ daemon: partial socket I/O (added `read_full`/`write_full` helpers)
- C++ daemon: async-unsafe signal handler (now uses `write()`)
- C++ daemon: SHA-256 byte-by-byte processing (now uses 64-byte blocks)
- C++ daemon: non-portable file time conversion (now uses C++20 `file_clock`)
- C++ daemon: missing `#include <cstdint>` for `uint32_t`
- Java frontend: IPC send/disconnect race (synchronized `disconnect()`)
- Java frontend: listener leak on window close (added `removeListener()`)
- Java frontend: zombie Go coordinator restart after 3 connection failures
- CI: macOS Sequoia dyld `LC_UUID` crash (fixed by using Go 1.23 runner)
- CI: flaky `TestAutoReconnect` timing (replaced sleep with poll)
