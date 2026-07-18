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
| Go coordinator | ✅ Works | Uses TCP socket IPC or fallback |
| Java frontend | ✅ Works | Java's socket API is cross-platform |
| C++ daemon | ✅ Works | Supports TCP loopback IPC natively |

## Decision

On Windows, we will use **TCP Loopback sockets** (`127.0.0.1`) instead of Win32 Named Pipes. This keeps the socket code extremely portable, utilizing standard cross-platform BSD sockets APIs across all three components (Go, C++, Java), and completely avoids complex Win32-specific APIs.

To support this cleanly and securely across testing environments and multiple parallel instances, we introduce the `IPC_TCP_PORT` environment variable. When `IPC_TCP_PORT` is specified:
1. The Go coordinator listens on `127.0.0.1:<IPC_TCP_PORT>` for local frontend/daemon connections.
2. The C++ daemon connects to `127.0.0.1:<IPC_TCP_PORT>` for IPC.

If `IPC_TCP_PORT` is not specified, the system continues to use **Unix Domain Sockets** (`/tmp/p2p_sync.sock`) on Linux and macOS as the default high-performance transport. On Windows, setting `IPC_TCP_PORT` (or falling back to a default port) is standard.

### IPC Transport by Platform

| Platform | Go ↔ C++ IPC  | Rationale |
|-----------|---------------|-----------|
| Linux     | Unix socket   | Best performance, native POSIX |
| macOS     | Unix socket   | Same as Linux |
| Windows   | TCP Loopback  | Simple, cross-platform socket code, highly robust |

### TCP Loopback Implementation

```cpp
// Common connection logic in C++ (src/backend/cpp/src/ipc_client.cpp)
#ifdef _WIN32
    // Windows socket initialization
    WSADATA wsaData;
    WSAStartup(MAKEWORD(2, 2), &wsaData);
#endif

    // If port is parsed from IPC_TCP_PORT environment variable, connect via TCP:
    int sock = socket(AF_INET, SOCK_STREAM, 0);
    struct sockaddr_in addr;
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);
    inet_pton(AF_INET, "127.0.0.1", &addr.sin_addr);
    connect(sock, (struct sockaddr*)&addr, sizeof(addr));
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
    if IPC_TCP_PORT environment variable is set:
        connect_tcp_socket("127.0.0.1", IPC_TCP_PORT)
    elif socket_path contains "/" or starts with ".":
        connect_unix_socket(socket_path)
    else:
        connect_tcp_socket("127.0.0.1", 9999)  // fallback
```

## Consequences

**Positive:**
- Full, native Windows support for the C++ daemon using standard BSD sockets APIs.
- Extreme simplicity: minimal platform-specific code. No complex overlapped I/O or multi-threaded named pipes code to maintain.
- Standardized cross-platform E2E network testing harness (using `IPC_TCP_PORT` to spin up isolated peers concurrently on the same host).
- Identical JSON-framed protocol (4-byte length prefix + JSON) across Unix Domain Sockets and TCP socket transports.

**Negative:**
- Port management: loopback TCP port must be assigned/configured securely (handled seamlessly via `IPC_TCP_PORT` for testing/production setups).

## Status

Fully Implemented and Released (v1.6.2). Native E2E integration tests are verified on Linux, macOS, and Windows.

## References
- Go coordinator TCP IPC implementation: `src/backend/go/pkg/ipc/ipc_server.go`
- C++ daemon TCP IPC implementation: `src/backend/cpp/src/ipc_client.cpp`
- Native Cross-Platform Test Harness: `scripts/integration_harness.py`