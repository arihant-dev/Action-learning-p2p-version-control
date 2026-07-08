---
name: industry-cpp-daemon
description: Cross-platform C++ daemon with native watchers, OpenSSL, unit tests, and Windows support.
---

# C++ Daemon Agent

**Role:** Rewrite the C++ daemon for cross-platform support, native performance, and testability
**Branch:** `ws/cpp-daemon` (branched from `master`)
**Merge target:** `industry-shipping`
**Phase:** 1

## Work Items (in priority order)

1. **Native Filesystem Watchers**
   - Refactor `FileSystemWatcher` into a platform-abstracted interface
   - **Linux:** Implement `InotifyWatcher` using `inotify_init()`, `inotify_add_watch()`, `read()` with timeout
     - Watch directories recursively (add watches for new subdirectories)
     - Handle `IN_CREATE`, `IN_MODIFY`, `IN_DELETE`, `IN_MOVED_FROM`, `IN_MOVED_TO`
     - Debounce events with 100ms timer to coalesce rapid changes
   - **macOS:** Implement `FSEventsWatcher` using `FSEventStreamCreate()`, `FSEventStreamScheduleWithRunLoop()`
     - Watch at volume level for kFSEventStreamCreateFlagFileEvents
     - Filter to watched directory paths
   - **Windows:** Implement `ReadDirectoryChangesWatcher` using `ReadDirectoryChangesW()` with I/O completion port
     - Watch recursively, buffer size large enough to avoid overflow
   - Keep `PollingWatcher` as compile-time fallback
   - Add CMake options: `USE_INOTIFY`, `USE_FSEVENTS`, `USE_READDIRECTORYCHANGES`, `USE_POLLING`

2. **Replace Custom SHA-256 with OpenSSL**
   - Use `EVP_MD_CTX_new()`, `EVP_DigestInit_ex(ctx, EVP_sha256(), NULL)`, `EVP_DigestUpdate()`, `EVP_DigestFinal_ex()`
   - Remove `sha256.cpp` and `sha256.h` (or keep as fallback)
   - Link `-lcrypto` in CMakeLists.txt
   - Add CMake option: `USE_OPENSSL` (default: ON)
   - Performance: hardware SHA acceleration on modern CPUs (Intel SHA-NI, ARM SHA extensions)

3. **Windows Port**
   - **IPC:** Replace Unix domain sockets with Windows Named Pipes (`CreateNamedPipe()` / `ConnectNamedPipe()` / `CallNamedPipe()`)
   - **TCP sockets:** Replace POSIX socket API with Winsock2 (`WSAStartup()`, `WSASocket()`, `connect()`, `send()`, `recv()`)
   - **Signal handling:** Replace `SIGTERM` handler with Windows console handler (`SetConsoleCtrlHandler()`)
   - **Process management:** Replace `getpid()`, `kill()` with Windows equivalents
   - **File operations:** Use `CreateFile()`, `ReadFile()`, `WriteFile()` with `\\?\` prefix for long paths
   - **File permissions:** Replace `chmod()` with Windows ACL (`SetSecurityInfo()`, `SetEntriesInAclA()`)
   - **Atomic rename:** Replace `rename()` with `MoveFileEx(MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)`
   - **Threading:** Replace `pthread_create()` with `_beginthreadex()` / `std::thread`
   - Add CMake conditionals: `WIN32`, `APPLE`, `LINUX`

4. **C++ Unit Tests (GoogleTest or Catch2)**
   - Set up GoogleTest submodule or FetchContent
   - Test files:
     - `test_ipc_client.cpp` — Connect, send, receive, reconnect
     - `test_filesystem_watcher.cpp` — File creation, modification, deletion events
     - `test_sha256.cpp` — Hash computation, empty file, large file
     - `test_file_transfer.cpp` — Upload, download, hash verification, atomic rename
   - Integrate with CTest in CMakeLists.txt
   - Aim for >70% line coverage

5. **Configurable Poll Interval**
   - Add `--poll-interval` / `P2P_POLL_INTERVAL_MS` config
   - Default: 1000ms (current behavior)
   - Used only when native watcher is unavailable
   - Minimum: 100ms, Maximum: 30000ms

6. **Windows File Permissions**
   - Implement `apply_file_permissions()` that handles both Unix and Windows
   - Unix: `chmod()` with mode from metadata (existing)
   - Windows: `SetFileSecurity()` with DACL translation
   - Handle permission inheritance in Windows directories

7. **Graceful Shutdown**
   - On SIGTERM (Unix) / Ctrl-C (Windows):
     - Flush pending file events to IPC
     - Wait for active file transfers to complete (with timeout)
     - Send shutdown acknowledgment to Go coordinator
     - Exit cleanly
   - On SIGKILL: best-effort flush (no guarantee)

## Relevant Files
- `src/backend/cpp/` — All C++ sources and headers
- `src/backend/cpp/CMakeLists.txt` — Build configuration
- `src/backend/cpp/src/sha256.cpp` — Replace with OpenSSL
- `src/backend/cpp/src/filesystem_watcher.cpp` — Replace implementation
- `src/backend/cpp/include/filesystem_watcher.h` — Abstract watcher interface
- `src/backend/cpp/src/ipc_client.cpp` — Windows named pipes
- `src/backend/cpp/include/ipc_client.h` — IPC interface
- `src/backend/cpp/src/file_transfer.cpp` — Windows file ops

## Verification
- `cd src/backend/cpp && cmake -B build && cmake --build build` — must compile on all platforms
- `cd src/backend/cpp && ctest --test-dir build --output-on-failure` — all tests pass
- Native watcher should detect file changes in <100ms
- OpenSSL SHA-256 should match custom implementation output
- Windows build should compile and run without POSIX dependencies
