# P2P File Sync Architecture

## Overview

A hybrid P2P file synchronization system combining Go (82% - networking, coordination, version control) and C++ (18% - file I/O optimization). The architecture prioritizes scalability, conflict resolution, and multi-repository support through language-specific strengths.

---

## Task Division

### System Composition
- **Go: 82% (13 tasks, ~5,500 LOC)**
- **C++: 18% (4 tasks, ~1,200 LOC)**

### Go Responsibilities ✓✓

#### Networking (4 tasks)
1. **Peer Discovery** - mDNS broadcasting, goroutines handle scaling
2. **TCP Connections** - 10K+ concurrent peers with goroutines
3. **Protocol Handling** - JSON serialization, built-in encoding
4. **Message Routing** - Channel-based orchestration, select statement

#### Version Control (4 tasks)
1. **Conflict Detection** - 3-way conflicts, simpler state logic
2. **Conflict Resolution** - Last-Write-Wins, backup older versions
3. **Vector Clocks** - Timestamp tracking, distributed systems
4. **Version History** - io.Copy, directory management, cleanup

#### Multi-Repo Coordination (3 tasks)
1. **Repo State Machine** - 5+ repos with independent sync
2. **Bandwidth Allocation** - Fair scheduling across repos
3. **Sync Prioritization** - Goroutines per repo, no OS overhead

#### Storage & IPC (2 tasks)
1. **SQLite Database** - Cleaner database/sql interface
2. **IPC Server** - Non-blocking message relay to Java/C++

### C++ Responsibilities ✓✓

#### File System Operations (4 tasks)
1. **File Watching** - Direct inotify/FSEvents, 5-15% faster
2. **File Hashing** - 15% faster on large files (256MB+), memory-mapped
3. **Atomic Operations** - Guaranteed rename atomicity, direct OS APIs
4. **Permissions** - Direct chmod/chown/stat access

---

## Component Scope & Boundaries

### Go Component Scope

**Responsibilities**:
- **Network Protocol**: All peer-to-peer communication, connection management, and protocol handling
- **Conflict Resolution**: Version control logic, conflict detection, and resolution strategies (Last-Write-Wins)
- **State Management**: Repository state machines, vector clocks, and distributed coordination
- **Metadata Storage**: SQLite database for file metadata, version history, and sync state
- **IPC Coordination**: Receives file change notifications from C++, orchestrates sync operations

**Boundaries**:
- Does NOT directly access file system operations (no file reading/writing)
- Does NOT perform file hashing or watching
- Does NOT handle atomic file operations or permissions
- Delegates all disk I/O to C++ component via IPC

**Key Principle**: Go manages "what" to sync and "when", but C++ handles "how" the data moves to/from disk.

### C++ Component Scope

**Responsibilities**:
- **File System Operations**: Direct OS API integration for file watching, hashing, and atomic operations
- **Data Transfer**: Receives file data from network sockets and applies to local disk
- **Conflict Application**: Executes conflict resolution decisions made by Go (backup, overwrite, merge)
- **Performance Optimization**: Memory-mapped file operations for large files (>256MB)
- **IPC Communication**: Sends file change events to Go, receives sync commands

**Boundaries**:
- Does NOT handle network communication or peer discovery
- Does NOT make conflict resolution decisions
- Does NOT manage repository state or version control logic
- Does NOT coordinate multi-peer synchronization

**Key Principle**: C++ executes "how" to apply changes, but Go decides "what" changes to apply.

---

## File Transfer & Data Flow

### Sync Protocol Overview

The synchronization process follows a two-phase approach:

1. **Metadata Phase** (Go): Resolve conflicts and determine what needs to be transferred
2. **Data Phase** (C++): Transfer actual file content through dedicated network sockets

### Phase 1: Conflict Resolution (Go)

```
File change detected → Metadata exchange → Conflict analysis → Resolution decision
```

- Go receives file change notifications from C++ via IPC
- Go broadcasts metadata to peers (hash, size, timestamp, vector clock)
- Peers compare versions and report conflicts
- Go applies conflict resolution strategy (Last-Write-Wins)
- Go determines which files need full transfer vs. delta sync

### Phase 2: Data Transfer (Go → C++ Socket Handover)

```
Go resolves diffs → Creates transfer socket → Passes socket to C++ → C++ receives data → Applies to disk
```

**Detailed Flow**:

1. **Go Creates Transfer Socket**
   ```
   // Go creates a listening socket for data transfer
   transferSocket := net.Listen("tcp", "127.0.0.1:0") // Auto-assign port
   port := transferSocket.Addr().(*net.TCPAddr).Port
   ```

2. **Go Sends Socket Info to C++**
   ```
   // IPC message to C++
   message := Message{
       Type: "prepare_file_transfer",
       Payload: map[string]interface{}{
           "path": "/home/user/sync/document.txt",
           "peer_id": "alice-laptop",
           "transfer_port": port,
           "expected_hash": "abc123...",
           "expected_size": 1048576,
       }
   }
   ipcServer.SendMessage(message)
   ```

3. **C++ Accepts Socket Connection**
   ```
   // C++ connects to the transfer socket
   transferSocket = connect("127.0.0.1", port);
   
   // Wait for data stream from Go
   receive_file_data(transferSocket, file_path, expected_hash);
   ```

4. **Go Handles Network Transfer**
   ```
   // Go accepts connection from C++
   conn, _ := transferSocket.Accept()
   
   // Go receives data from peer network
   peerConn := getPeerConnection(peerID)
   data := receiveFromPeer(peerConn, fileSize)
   
   // Go streams data to C++ via socket
   conn.Write(data)
   ```

5. **C++ Applies Data to Disk**
   ```
   // C++ receives data stream
   while (bytes_received < expected_size) {
       bytes = read_from_socket(transferSocket);
       write_to_file(file_path, bytes);
   }
   
   // Verify hash and handle conflicts
   actual_hash = calculate_file_hash(file_path);
   if (actual_hash != expected_hash) {
       handle_real_conflict(file_path, expected_hash);
   }
   ```

### Real Conflict Resolution (C++)

When C++ detects a real conflict during data application:

```
C++ detects hash mismatch → Creates backup → Applies resolution strategy → Reports to Go
```

**Conflict Types Handled by C++**:
- **Hash Mismatch**: File content doesn't match expected hash
- **Permission Denied**: Cannot write to target location
- **Disk Full**: Insufficient space for file
- **File Locked**: Another process has file open

**Resolution Strategies**:
1. **Backup & Overwrite**: Create `.backup` file, apply new version
2. **Atomic Rename**: Use OS atomic operations for safe replacement
3. **Partial Recovery**: Revert to last known good state
4. **User Notification**: Report unresolvable conflicts to Go for user decision

### Socket Management

**Socket Lifecycle**:
- Go creates one socket per file transfer
- Socket is passed to C++ via IPC message
- C++ connects and receives data stream
- Socket is closed after transfer completion
- Failed transfers trigger retry logic in Go

**Performance Benefits**:
- **Zero-copy**: Data flows directly from network to disk
- **Memory efficient**: No intermediate buffering in Go
- **Concurrent**: Multiple file transfers can run simultaneously
- **Error isolation**: Network errors don't affect IPC communication

---

## Architectural Rationale

### Why Go for Network Operations?

| Criterion              | Go                     | C++                     |
| ---------------------- | ---------------------- | ----------------------- |
| Concurrent connections | 10K goroutines (cheap) | 10K threads (expensive) |
| TCP server complexity  | 50 lines               | 300+ lines              |
| Error handling         | Explicit, clear        | Exceptions, errno       |
| Testing async code     | Easy (goroutines)      | Hard (threads)          |
| **Verdict**            | ✓✓ Excellent           | ✗ Painful               |

### Why Go for Version Control & Coordination?

| Criterion               | Go                     | C++                     |
| ----------------------- | ---------------------- | ----------------------- |
| State management        | Maps/slices            | Pointers/structs        |
| Conflict logic          | Simple conditionals    | Complex state machines  |
| Multi-repo coordination | Channels (no deadlock) | Mutexes (deadlock risk) |
| Testing coordination    | Easy                   | Very hard               |
| **Verdict**             | ✓✓ Excellent           | ✗ Error-prone           |

### Why C++ for File I/O?

| Criterion      | Go                             | C++                |
| -------------- | ------------------------------ | ------------------ |
| Direct OS APIs | No (via library)               | Yes (direct)       |
| Performance    | Sufficient (but 10-15% slower) | Optimal (direct)   |
| Use case       | Typical files <100MB           | Large files >500MB |
| **Verdict**    | ✓ Good                         | ✓✓ Better          |

---

## IPC Communication Points

### Direction: C++ → Go

```
File changed (detected by C++ watcher)
    ↓
C++ sends to Go: {type: "file_changed", path: "...", hash: "...", size: ...}
    ↓
Go routes to peers via network
    ↓
Peers send back acknowledgment/conflict
    ↓
Go sends to C++: {type: "conflict_detected", ...} or {type: "sync_complete"}
```

### Direction: Go → C++

```
Network receives file from peer
    ↓
Go sends to C++: {type: "sync_from_peer", path: "...", content: "...", hash: "..."}
    ↓
C++ applies file locally (or detects conflict)
    ↓
C++ sends to Go: {type: "file_applied"} or {type: "conflict_detected"}
```

### IPC Characteristics
- **Frequency**: ~50-100 messages per minute per active peer
- **Latency**: ~1-2ms per message (negligible vs 50-100ms network)
- **Impact**: IPC overhead is <1% of total network transfer time

---

## Component Architecture

### C++ Daemon Components

```
┌─────────────────────────────┐
│     File System Watcher     │
│  (inotify/FSEvents/native)  │
└──────────────┬──────────────┘
               │
               ├─► Hash Manager
               │   (SHA256, memory-mapped)
               │
               ├─► Repository Manager
               │   (Multiple repos)
               │
               └─► IPC Client
                   (Unix socket/TCP)
                        │
                        ↓ (JSON messages)
                  ┌─────────────┐
                  │ Go Network  │
                  │  Component  │
                  └─────────────┘
```

### Go Network Components

```
┌─────────────────────────────────┐
│      Peer Discovery (mDNS)      │
└──────────────┬──────────────────┘
               │
               ├─► Connection Manager
               │   (TCP multiplexing)
               │
               ├─► Version Control Engine
               │   (Conflict detection/resolution)
               │
               ├─► Sync Coordinator
               │   (Multi-repo state machine)
               │
               ├─► SQLite Database
               │   (State persistence)
               │
               └─► IPC Server
                   (Unix socket/TCP)
                        │
                        ↓ (JSON messages)
                  ┌──────────────┐
                  │ C++ File I/O │
                  │   Component  │
                  └──────────────┘
```

---

## Performance Impact Analysis

### Worst-Case Scenario: 256MB File with Conflict

```
Operation                              Time      Overhead
───────────────────────────────────────────────────────────
1. File changed (detected by C++)       2ms       -
2. C++ → Go IPC                         1ms       Go: channels
3. Go routes to 2 peers                 3ms       Go: network mux
4. Network transfer (256MB)           2000ms      Network (bottleneck)
5. Peer detects change                  2ms       -
6. Peer checks local version            1ms       -
7. Peer → Go conflict detection        2ms       Go: channels
8. Go → C++ IPC (conflict)             1ms       Go: channels
9. C++ decides resolution               1ms       -
10. Apply/backup decision              10ms      -
───────────────────────────────────────────────────────────
Total Time                           ~2000ms
IPC Overhead                           ~8ms      (0.4% of total)
Network Overhead                    ~1950ms      (97.5% of total)
```

**Key Insight**: Network is the bottleneck, not IPC or computation.

---

## Data Flow Examples

### Multi-Repo Coordination in Go

```go
// Clean, non-blocking multi-repo coordination
func (rc *RepoCoordinator) coordinateSync(ctx context.Context) {
    for {
        select {
        case conflict := <-repoA.ConflictChan:
            rc.resolveConflict(conflict)
        case conflict := <-repoB.ConflictChan:
            rc.resolveConflict(conflict)
        case transfer := <-repoC.TransferReadyChan:
            rc.startTransfer(transfer)
        case <-ctx.Done():
            return
        }
    }
}
// Result: Fair scheduling, no deadlock risk, scales to N repos
```

### File Watching in C++

```cpp
// Direct OS API access for optimal performance
class FileSystemWatcher {
private:
#ifdef __linux__
    int inotify_fd_;     // Direct inotify file descriptor
    int watch_descriptor_;
#elif __APPLE__
    FSEventStreamRef stream_ref_;  // Direct FSEvents
#elif _WIN32
    HANDLE dir_handle_;  // Direct Windows API
#endif

    // Platform-specific native implementations
    void handle_inotify_events();   // Linux: Direct event loop
    void handle_fsevents();          // macOS: FSEvents callback
    void handle_file_changes();      // Windows: ReadDirectoryChangesW
};
```

---

## Integration Points

### 1. File Change Detection Flow
- C++ detects file change via inotify/FSEvents
- C++ calculates file hash (optimized with mmap)
- C++ sends to Go: `{type: "file_changed", path, hash, size}`
- Go stores in SQLite and determines sync action

### 2. Conflict Resolution Flow
- Go detects conflict (version mismatch)
- Go sends to C++: `{type: "conflict_detected", path, versions}`
- C++ applies resolution (keep local/remote/backup)
- C++ sends to Go: `{type: "resolution_applied"}`

### 3. Multi-Repository Sync
- Go maintains independent state machine per repository
- Go allocates bandwidth fairly across repos
- Go uses channel-based coordination (no mutex bottleneck)
- C++ manages file operations independently per repo

---

## Technology Stack

### Go Ecosystem
- **Networking**: net, encoding/json
- **Concurrency**: goroutines, channels, context
- **Service Discovery**: zeroconf (mDNS)
- **Database**: database/sql with SQLite3 driver
- **IPC**: Unix sockets (Linux/macOS) or TCP (Windows)

### C++ Ecosystem
- **File Watching**: 
  - Linux: inotify API
  - macOS: FSEvents API
  - Windows: ReadDirectoryChangesW
- **Hashing**: OpenSSL or libsodium (SHA256)
- **IPC**: Unix sockets (SOCK_STREAM) or TCP
- **Build**: CMake 3.10+

---

## Scalability Considerations

### Handling 10K+ Peers
- **Go goroutines**: Lightweight (<1KB each), suitable for 10K+ connections
- **C++ threads**: Heavyweight (1-2MB each), not practical for 10K+
- **Resolution**: Go handles all peer connections; C++ handles file I/O only

### Large File Support (256MB+)
- **C++ memory mapping**: Direct OS paging, minimal memory overhead
- **Go streaming**: Process chunks, less RAM but slower
- **Trade-off**: Let C++ hash large files; Go orchestrates transfer

### Multi-Repository Coordination
- **Independent state machines**: Each repo progresses independently
- **Fair scheduling**: Select statement in Go ensures fairness
- **No bottlenecks**: No centralized mutex or lock contention

---

## Deployment Architecture

```
┌─────────────────────────────────────────────────────┐
│              Node A (User's Computer)               │
├─────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────┐ │
│ │ C++ Daemon (File I/O)                           │ │
│ │ - Watches: /home/user/sync                      │ │
│ │ - Hashes files                                  │ │
│ │ - Applies changes locally                       │ │
│ └────────────────┬────────────────────────────────┘ │
│                  │                                   │
│                  ↓ IPC (Unix socket /tmp/p2p.sock)   │
│                  │                                   │
│ ┌────────────────┴────────────────────────────────┐ │
│ │ Go Network Daemon                               │ │
│ │ - Discovers peers via mDNS                      │ │
│ │ - Manages TCP connections                       │ │
│ │ - Handles conflicts (LWW)                       │ │
│ │ - Stores metadata in SQLite                     │ │
│ └────────────────┬────────────────────────────────┘ │
│                  │                                   │
│                  ↓ Network (TCP 9876)                │
└──────────────────┼─────────────────────────────────┘
                   │
       ┌───────────┼───────────┐
       ↓           ↓           ↓
    [Node B]   [Node C]   [Node D]
     (Peers)    (Peers)    (Peers)
```

---

## Summary

This architecture leverages each language's strengths:

- **Go (82%)**: Handles network complexity, concurrency, and coordination elegantly
- **C++ (18%)**: Optimizes critical file I/O operations with direct OS APIs
- **IPC**: Minimal overhead (<1% of system time) despite frequent messages
- **Scalability**: Supports 10K+ peers without thread explosion
- **Reliability**: Channel-based coordination eliminates deadlock risks

The design prioritizes network and application logic in a language optimized for both, while delegating performance-critical file operations to a language optimized for OS integration.
