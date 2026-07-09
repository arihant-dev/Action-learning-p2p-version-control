# ADR-005: Windows Port — Named Pipes vs Unix Sockets

**Date:** 2026-07-08
**Context:** Architecture Team

---

## Context

The C++ daemon currently uses Unix domain sockets (`AF_UNIX`) for IPC with
the Go coordinator. This is not available on Windows. Additionally, the C++
sources use POSIX-only headers (`<sys/socket.h>`, `<sys/un.h>`, `<unistd.h>`).

The three components have different Windows readiness:

| Component | Windows Status | Issue |
|-----------|---------------|------|
| Go coordinator | ✅ Works | Uses TCP fallback for IPC (`127.0.0.1:9999`) |
| Java frontend | ✅ Works | Java's socket API is cross-platform |
| C++ daemon | ❌ Broken | Uses `AF_UNIX`, POSIX headers |

## Decision

On Windows, we will replace the IPC transport from **Unix sockets** to **Named Pipes**
for the C++ daemon. The Go coordinator already falls back to TCP loopback,
but Named Pipes offer better performance and integration with Windows.

### IPC Transport by Platform

| Platform | Go ↔ C++ IPC  | Rationale |
|-----------|---------------|-----------|
| Linux     | Unix socket   | Best performance, native POSIX |
| macOS     | Unix socket   | Same as Linux |
| Windows   | Named pipe    | Windows-native, integrated with security descriptors |

### Named Pipe Implementation (Windows)

```cpp
#ifdef _WIN32
    // Server side (Go):
    // Go creates a named pipe: \\.\pipe\p2p_sync
    
    // Client side (C++):
    HANDLE pipe = CreateFile(
        L"\\\\.\\pipe\\p2p_sync",
        GENERIC_READ | GENERIC_WRITE,
        FILE_SHARE_READ | FILE_SHARE_WRITE,
        NULL,
        OPEN_EXISTING,
        FILE_ATTRIBUTE_NORMAL,
        NULL
    );
    
    // Use ReadFile/WriteFile instead of read/write
    // Use OVERLAPPED for async I/O
#endif
```

### Build System Changes

```cmake
if(WIN32)
    target_link_libraries(cpp_daemon ws2_32)
    set(PLATFORM_SOURCES
        src/ipc/ipc_client_windows.cpp
        src/fs/watcher_windows.cpp
    )
endif()
```

### Socket Fallback Priority

```
ipc_client::connect(socket_path):
    if Windows AND socket_path contains "\\pipe\\":
        connect_named_pipe(socket_path)
    elif socket_path contains "/":
        connect_unix_socket(socket_path)
    else:
        connect_tcp_socket("127.0.0.1", 9999)  // fallback
```

## Consequences

**Positive:**
- Full Windows support for the C++ daemon
- Named Pipes have better performance than TCP loopback (kernel-level)
- Windows security model (DACL) can restrict pipe access
- Same framing protocol (4-byte length prefix + JSON) as Unix sockets

**Negative:**
- Additional code to maintain (platform-specific IPC client)
- Named Pipes require Async I/O or overlapped I/O for non-blocking
- Testing requires actual Windows environments
- Different error handling patterns (`GetLastError()` vs `errno`)

## Status

Accepted. Windows port is scheduled for Phase 2.

## References
- Knowledge Graph: Windows porting requirements
- IPC Protocol Specification: Fallback transport