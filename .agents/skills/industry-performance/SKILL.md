---
name: industry-performance
description: Optimization, native watchers, hardware-accelerated crypto, compression, and delta sync.
---

# Performance Agent

**Role:** Optimize resource usage, add native platform support, and reduce latency
**Branch:** `ws/performance` (branched from `master`)
**Merge target:** `industry-shipping`
**Phase:** 2 (depends on Go Coordinator, C++ Daemon)

## Work Items (in priority order)

1. **Native Filesystem Watchers (C++)**
   - **Linux:** Replace polling with `inotify` (watch directory trees via `IN_CREATE`, `IN_MODIFY`, `IN_DELETE`, `IN_MOVED_FROM`, `IN_MOVED_TO`)
   - **macOS:** Replace polling with `FSEvents` API (stream of file system events at the volume level)
   - **Windows:** Use `ReadDirectoryChangesW` with I/O completion ports
   - Keep polling as fallback when native watcher unavailable
   - Coalesce rapid events (debounce timer: 100ms)

2. **Replace Custom SHA-256 with OpenSSL (C++)**
   - Use `EVP_DigestInit_ex` / `EVP_DigestUpdate` / `EVP_DigestFinal_ex` from OpenSSL
   - Hardware acceleration: SHA-NI instructions used automatically on supporting CPUs
   - Link: `-lcrypto` on all platforms
   - Keep custom implementation as compile-time fallback (configurable via CMake flag)

3. **Bounded Broadcast (Go)**
   - Current: `Broadcast()` spawns unbounded goroutines for each connected peer
   - Fix: use a semaphore-controlled worker pool (configurable size, default 100)
   - If worker pool full, queue the broadcast or drop with metric increment
   - Prevent OOM under high peer count

4. **Benchmark Suite (Go)**
   - Add benchmarks in each package:
     - `BenchmarkBroadcast` — P2P message broadcast throughput
     - `BenchmarkHandshake` — Connection setup latency
     - `BenchmarkFileTransfer` — Transfer throughput with various file sizes
     - `BenchmarkConflictDetection` — Conflict resolution speed
     - `BenchmarkSQLiteRead/Write` — Storage layer throughput
   - Run: `go test -bench=. -benchmem ./...`

5. **pprof Endpoints (Go)**
   - Add `/debug/pprof/` endpoints to HTTP server
   - Enable CPU, memory, goroutine, mutex profiling at runtime
   - Document how to collect profiles

6. **zstd Compression for P2P Transfers**
   - Add optional zstd compression for file transfer data plane
   - Configurable compression level (1-22, default 3 for speed)
   - Negotiate compression capability during P2P handshake
   - Auto-detect: compress files > configurable threshold (default 1MB)
   - Use Go `github.com/klauspost/compress/zstd`

7. **Delta Sync (rsync-style)**
   - For large files (>100MB), implement rolling hash-based delta algorithm
   - Split file into fixed-size blocks (default 8KB)
   - Compute rolling checksums (weak Adler-32 + strong SHA-256 per block)
   - Transfer only changed blocks
   - C++ daemon reassembles file from local base + received blocks

## Relevant Files
- `src/backend/cpp/src/filesystem_watcher.cpp` — Polling watcher (replace with native)
- `src/backend/cpp/src/sha256.cpp` — Custom SHA-256 (replace with OpenSSL)
- `src/backend/cpp/CMakeLists.txt` — Add OpenSSL dependency
- `src/backend/cpp/include/filesystem_watcher.h` — Watcher interface
- `src/backend/go/pkg/network/connection_manager.go` — Broadcast
- `src/backend/go/main.go` — HTTP server (add pprof)
- `src/backend/go/pkg/transfer/file_transfer.go` — Compression
- `src/backend/go/pkg/protocol/messages.go` — Compression negotiation
- `src/backend/cpp/src/file_transfer.cpp` — C++ delta sync

## Verification
- `cd src/backend/cpp && cmake -B build && cmake --build build` — must compile
- `cd src/backend/go && go test -bench=. -benchmem ./...` — benchmarks run
- Native watcher detects file changes < 100ms (vs 1s polling)
- SHA-256 performance improvement measurable via benchmark
- Compression reduces transfer size by >50% for text files
- Delta sync only transfers changed blocks for modified large files
