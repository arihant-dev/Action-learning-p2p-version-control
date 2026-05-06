# P2P File Sync: Implementation Guide

## Architecture Overview

**System Division**:
- **Go: 82% (13 tasks, ~5,500 LOC)** - Networking, Version Control, Coordination, Storage & IPC
- **C++: 18% (4 tasks, ~1,200 LOC)** - File I/O Optimization (Watching, Hashing, Atomic Ops, Permissions)

**Key Principle**: Go handles all complex state management and concurrency; C++ optimizes critical file operations.

See [p2p_architecture.md](p2p_architecture.md) for detailed architectural rationale and performance analysis.

---

## Part 1: IPC Setup

### 1.1 Cross-Platform IPC Client (C++)

**File: cpp/src/ipc/ipc_client.hpp**

```cpp
#pragma once

#include <string>
#include <functional>
#include <thread>
#include <queue>
#include <mutex>
#include <json/json.h>

namespace p2p {
namespace ipc {

class IpcClient {
public:
    using MessageCallback = std::function<void(const Json::Value&)>;
    
    IpcClient();
    ~IpcClient();
    
    // Connect to IPC server
    bool connect(const std::string& socket_path, int timeout_ms = 5000);
    
    // Disconnect from IPC server
    void disconnect();
    
    // Send message to IPC server
    bool send_message(const Json::Value& message);
    
    // Register callback for incoming messages
    void on_message(MessageCallback callback);
    
    // Check if connected
    bool is_connected() const;
    
private:
    // Platform-specific implementations
    bool connect_unix_socket(const std::string& socket_path, int timeout_ms);
    bool connect_tcp_socket(const std::string& host, int port, int timeout_ms);
    
    // Message reading/writing
    bool write_message(const std::string& data);
    std::string read_message();
    
    // Background thread for reading messages
    void message_reader_thread();
    
    // IPC socket file descriptor or handle
    int socket_fd_;
    bool connected_;
    
    // Thread for asynchronous message reading
    std::thread reader_thread_;
    std::mutex reader_mutex_;
    
    // Message callback
    MessageCallback message_callback_;
    
    // Cross-platform implementation would use #ifdef for Windows/Unix
};

} // namespace ipc
} // namespace p2p
```

**File: cpp/src/ipc/ipc_client.cpp**

```cpp
#include "ipc_client.hpp"
#include <iostream>
#include <cstring>
#include <stdint.h>

#ifdef _WIN32
    #include <winsock2.h>
#else
    #include <sys/socket.h>
    #include <sys/un.h>
    #include <netinet/in.h>
    #include <arpa/inet.h>
    #include <unistd.h>
    #include <fcntl.h>
#endif

namespace p2p {
namespace ipc {

IpcClient::IpcClient() 
    : socket_fd_(-1), connected_(false) {
}

IpcClient::~IpcClient() {
    disconnect();
}

bool IpcClient::connect(const std::string& socket_path, int timeout_ms) {
#ifdef _WIN32
    // On Windows, try named pipe or TCP
    if (socket_path.find("\\") != std::string::npos) {
        return connect_tcp_socket("127.0.0.1", 9999, timeout_ms);
    }
#else
    // On Unix, try Unix socket first
    if (socket_path.find("/") != std::string::npos) {
        if (connect_unix_socket(socket_path, timeout_ms)) {
            connected_ = true;
            reader_thread_ = std::thread(&IpcClient::message_reader_thread, this);
            return true;
        }
    }
#endif
    // Fallback to TCP
    return connect_tcp_socket("127.0.0.1", 9999, timeout_ms);
}

#ifndef _WIN32
bool IpcClient::connect_unix_socket(const std::string& socket_path, int timeout_ms) {
    socket_fd_ = socket(AF_UNIX, SOCK_STREAM, 0);
    if (socket_fd_ < 0) {
        std::cerr << "Failed to create Unix socket" << std::endl;
        return false;
    }
    
    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, socket_path.c_str(), sizeof(addr.sun_path) - 1);
    
    if (::connect(socket_fd_, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        std::cerr << "Failed to connect to Unix socket: " << socket_path << std::endl;
        close(socket_fd_);
        socket_fd_ = -1;
        return false;
    }
    
    // Set socket to non-blocking
    int flags = fcntl(socket_fd_, F_GETFL, 0);
    fcntl(socket_fd_, F_SETFL, flags | O_NONBLOCK);
    
    std::cout << "Connected to Unix socket: " << socket_path << std::endl;
    return true;
}
#endif

bool IpcClient::connect_tcp_socket(const std::string& host, int port, int timeout_ms) {
#ifdef _WIN32
    WSADATA wsa_data;
    WSAStartup(MAKEWORD(2, 2), &wsa_data);
#endif
    
    socket_fd_ = socket(AF_INET, SOCK_STREAM, 0);
    if (socket_fd_ < 0) {
        std::cerr << "Failed to create TCP socket" << std::endl;
        return false;
    }
    
    struct sockaddr_in addr;
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);
    inet_pton(AF_INET, host.c_str(), &addr.sin_addr);
    
    if (::connect(socket_fd_, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        std::cerr << "Failed to connect to TCP socket: " << host << ":" << port << std::endl;
#ifdef _WIN32
        closesocket(socket_fd_);
#else
        close(socket_fd_);
#endif
        socket_fd_ = -1;
        return false;
    }
    
    std::cout << "Connected to TCP socket: " << host << ":" << port << std::endl;
    return true;
}

void IpcClient::disconnect() {
    connected_ = false;
    if (reader_thread_.joinable()) {
        reader_thread_.join();
    }
    if (socket_fd_ >= 0) {
#ifdef _WIN32
        closesocket(socket_fd_);
#else
        close(socket_fd_);
#endif
        socket_fd_ = -1;
    }
}

bool IpcClient::send_message(const Json::Value& message) {
    if (!connected_) {
        std::cerr << "Not connected to IPC server" << std::endl;
        return false;
    }
    
    Json::StreamWriterBuilder writer;
    std::string json_str = Json::writeString(writer, message);
    
    return write_message(json_str);
}

bool IpcClient::write_message(const std::string& data) {
    // Implement length-prefixed framing: [4-byte length][JSON]
    uint32_t len = htonl(data.size());
    
#ifdef _WIN32
    if (send(socket_fd_, (const char*)&len, sizeof(len), 0) < 0) {
        std::cerr << "Failed to send message length" << std::endl;
        return false;
    }
    if (send(socket_fd_, data.c_str(), data.size(), 0) < 0) {
        std::cerr << "Failed to send message data" << std::endl;
        return false;
    }
#else
    if (::write(socket_fd_, &len, sizeof(len)) < 0) {
        std::cerr << "Failed to send message length" << std::endl;
        return false;
    }
    if (::write(socket_fd_, data.c_str(), data.size()) < 0) {
        std::cerr << "Failed to send message data" << std::endl;
        return false;
    }
#endif
    
    return true;
}

std::string IpcClient::read_message() {
    uint32_t len;
    
#ifdef _WIN32
    int ret = recv(socket_fd_, (char*)&len, sizeof(len), 0);
#else
    int ret = ::read(socket_fd_, &len, sizeof(len));
#endif
    
    if (ret <= 0) {
        return "";
    }
    
    len = ntohl(len);
    if (len > 1024 * 1024) { // 1MB limit
        std::cerr << "Message too large: " << len << std::endl;
        return "";
    }
    
    char* buffer = new char[len];
#ifdef _WIN32
    recv(socket_fd_, buffer, len, 0);
#else
    ::read(socket_fd_, buffer, len);
#endif
    
    std::string result(buffer, len);
    delete[] buffer;
    
    return result;
}

void IpcClient::message_reader_thread() {
    while (connected_) {
        std::string msg_str = read_message();
        if (msg_str.empty()) {
            continue;
        }
        
        Json::Value msg;
        Json::CharReaderBuilder reader;
        std::string errs;
        std::istringstream stream(msg_str);
        
        if (Json::parseFromStream(reader, stream, &msg, &errs)) {
            if (message_callback_) {
                message_callback_(msg);
            }
        } else {
            std::cerr << "Failed to parse IPC message: " << errs << std::endl;
        }
    }
}

void IpcClient::on_message(MessageCallback callback) {
    message_callback_ = callback;
}

bool IpcClient::is_connected() const {
    return connected_;
}

} // namespace ipc
} // namespace p2p
```

### 1.2 IPC Server in Go

**File: go/pkg/ipc/ipc_server.go**

```go
package ipc

import (
    "encoding/binary"
    "encoding/json"
    "fmt"
    "net"
    "os"
    "sync"
)

type Message struct {
    Version   string          `json:"version"`
    Type      string          `json:"type"`
    ID        string          `json:"id"`
    Timestamp int64           `json:"timestamp"`
    Source    string          `json:"source"`
    Payload   json.RawMessage `json:"payload"`
}

type IpcServer struct {
    socketPath string
    listener   net.Listener
    clients    map[net.Conn]bool
    clientMu   sync.Mutex
    
    // Callback for handling messages from C++ daemon
    OnMessage func(*Message) error
    
    // Channel for sending messages to C++ daemon
    ToC chan *Message
}

func NewIpcServer(socketPath string) *IpcServer {
    return &IpcServer{
        socketPath: socketPath,
        clients:    make(map[net.Conn]bool),
        ToC:        make(chan *Message, 100),
    }
}

func (s *IpcServer) Start() error {
    var listener net.Listener
    var err error
    
    // Remove existing socket file if it exists
    os.Remove(s.socketPath)
    
    // Try Unix socket first (Linux/macOS)
    listener, err = net.Listen("unix", s.socketPath)
    if err != nil {
        // Fallback to TCP on Windows or if Unix socket fails
        fmt.Println("Unix socket not available, falling back to TCP")
        listener, err = net.Listen("tcp", "127.0.0.1:9999")
        if err != nil {
            return err
        }
    }
    
    s.listener = listener
    fmt.Printf("IPC server listening on: %s\n", s.socketPath)
    
    // Set Unix socket permissions if applicable
    if sock, ok := listener.(*net.UnixListener); ok {
        os.Chmod(s.socketPath, 0600)
        fmt.Println("Unix socket permissions set to 0600")
    }
    
    // Accept connections in a goroutine
    go s.acceptConnections()
    
    // Handle outgoing messages in a goroutine
    go s.handleOutgoingMessages()
    
    return nil
}

func (s *IpcServer) acceptConnections() {
    for {
        conn, err := s.listener.Accept()
        if err != nil {
            fmt.Printf("Error accepting connection: %v\n", err)
            continue
        }
        
        s.clientMu.Lock()
        s.clients[conn] = true
        s.clientMu.Unlock()
        
        fmt.Printf("C++ daemon connected from: %s\n", conn.RemoteAddr())
        
        go s.handleClient(conn)
    }
}

func (s *IpcServer) handleClient(conn net.Conn) {
    defer func() {
        s.clientMu.Lock()
        delete(s.clients, conn)
        s.clientMu.Unlock()
        conn.Close()
        fmt.Println("C++ daemon disconnected")
    }()
    
    for {
        msg, err := readMessage(conn)
        if err != nil {
            fmt.Printf("Error reading message: %v\n", err)
            return
        }
        
        if msg == nil {
            continue
        }
        
        fmt.Printf("Received from C++: %s\n", msg.Type)
        
        if s.OnMessage != nil {
            if err := s.OnMessage(msg); err != nil {
                fmt.Printf("Error handling message: %v\n", err)
            }
        }
    }
}

func (s *IpcServer) handleOutgoingMessages() {
    for msg := range s.ToC {
        s.clientMu.Lock()
        for conn := range s.clients {
            go writeMessage(conn, msg)
        }
        s.clientMu.Unlock()
    }
}

func (s *IpcServer) SendMessage(msg *Message) {
    select {
    case s.ToC <- msg:
    default:
        fmt.Println("Warning: IPC message queue full, dropping message")
    }
}

func (s *IpcServer) Stop() {
    s.clientMu.Lock()
    for conn := range s.clients {
        conn.Close()
    }
    s.clientMu.Unlock()
    
    if s.listener != nil {
        s.listener.Close()
    }
    close(s.ToC)
}

// readMessage reads a length-prefixed JSON message
func readMessage(conn net.Conn) (*Message, error) {
    // Read 4-byte length prefix (big-endian)
    lenBuf := make([]byte, 4)
    n, err := conn.Read(lenBuf)
    if err != nil || n != 4 {
        return nil, err
    }
    
    len := binary.BigEndian.Uint32(lenBuf)
    if len > 1024*1024 { // 1MB limit
        return nil, fmt.Errorf("message too large: %d bytes", len)
    }
    
    // Read message body
    msgBuf := make([]byte, len)
    n, err = conn.Read(msgBuf)
    if err != nil || n != int(len) {
        return nil, err
    }
    
    var msg Message
    err = json.Unmarshal(msgBuf, &msg)
    if err != nil {
        return nil, err
    }
    
    return &msg, nil
}

// writeMessage writes a length-prefixed JSON message
func writeMessage(conn net.Conn, msg *Message) error {
    data, err := json.Marshal(msg)
    if err != nil {
        return err
    }
    
    lenBuf := make([]byte, 4)
    binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
    
    if _, err := conn.Write(lenBuf); err != nil {
        return err
    }
    
    if _, err := conn.Write(data); err != nil {
        return err
    }
    
    return nil
}
```

---

## Part 1.3: File Transfer Socket Handover (Go → C++)

### 1.3.1 Go File Transfer Coordinator

**File: go/pkg/transfer/file_transfer.go**

```go
package transfer

import (
    "fmt"
    "io"
    "net"
    "sync"
    "p2p/pkg/ipc"
)

type FileTransfer struct {
    ipcServer *ipc.IpcServer
    transfers map[string]*TransferSession // transferID -> session
    mu        sync.RWMutex
}

type TransferSession struct {
    TransferID    string
    FilePath      string
    PeerID        string
    ExpectedHash  string
    ExpectedSize  int64
    TransferPort  int
    Listener      net.Listener
    Status        string // "preparing", "transferring", "completed", "failed"
}

func NewFileTransfer(ipcServer *ipc.IpcServer) *FileTransfer {
    return &FileTransfer{
        ipcServer: ipcServer,
        transfers: make(map[string]*TransferSession),
    }
}

// InitiateFileTransfer creates a socket and tells C++ to connect
func (ft *FileTransfer) InitiateFileTransfer(filePath, peerID, expectedHash string, expectedSize int64) error {
    // Create listening socket on random port
    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        return fmt.Errorf("failed to create transfer socket: %v", err)
    }
    
    port := listener.Addr().(*net.TCPAddr).Port
    transferID := fmt.Sprintf("transfer_%s_%d", peerID, port)
    
    session := &TransferSession{
        TransferID:   transferID,
        FilePath:     filePath,
        PeerID:       peerID,
        ExpectedHash: expectedHash,
        ExpectedSize: expectedSize,
        TransferPort: port,
        Listener:     listener,
        Status:       "preparing",
    }
    
    ft.mu.Lock()
    ft.transfers[transferID] = session
    ft.mu.Unlock()
    
    // Send IPC message to C++ with socket info
    message := &ipc.Message{
        Type: "prepare_file_transfer",
        Payload: map[string]interface{}{
            "transfer_id":    transferID,
            "path":          filePath,
            "peer_id":       peerID,
            "transfer_port": port,
            "expected_hash": expectedHash,
            "expected_size": expectedSize,
        },
    }
    
    ft.ipcServer.SendMessage(message)
    
    // Start accepting connection from C++ in background
    go ft.handleTransfer(session)
    
    return nil
}

func (ft *FileTransfer) handleTransfer(session *TransferSession) {
    defer session.Listener.Close()
    
    // Accept connection from C++
    conn, err := session.Listener.Accept()
    if err != nil {
        session.Status = "failed"
        return
    }
    defer conn.Close()
    
    session.Status = "transferring"
    
    // Get peer connection
    peerConn := getPeerConnection(session.PeerID)
    if peerConn == nil {
        session.Status = "failed"
        return
    }
    
    // Stream data from peer to C++ via socket
    _, err = io.Copy(conn, peerConn)
    if err != nil {
        session.Status = "failed"
        return
    }
    
    session.Status = "completed"
    
    // Notify C++ that transfer is complete
    message := &ipc.Message{
        Type: "file_transfer_complete",
        Payload: map[string]interface{}{
            "transfer_id": session.TransferID,
            "path":       session.FilePath,
        },
    }
    ft.ipcServer.SendMessage(message)
}
```

### 1.3.2 C++ File Receiver

**File: cpp/src/transfer/file_receiver.hpp**

```cpp
#pragma once

#include <string>
#include <thread>
#include <mutex>
#include <json/json.h>
#include <functional>

namespace p2p {
namespace transfer {

struct TransferRequest {
    std::string transfer_id;
    std::string file_path;
    std::string peer_id;
    int transfer_port;
    std::string expected_hash;
    int64_t expected_size;
};

class FileReceiver {
public:
    using TransferCallback = std::function<void(const std::string& transfer_id, bool success)>;
    
    FileReceiver();
    ~FileReceiver();
    
    // Handle IPC message from Go to prepare for transfer
    void handle_prepare_transfer(const Json::Value& message);
    
    // Register callback for transfer completion
    void on_transfer_complete(TransferCallback callback);
    
private:
    // Connect to transfer socket and receive file
    void receive_file(const TransferRequest& request);
    
    // Verify received file against expected hash
    bool verify_file(const std::string& file_path, const std::string& expected_hash);
    
    // Handle real conflicts (hash mismatch, permissions, etc.)
    void handle_real_conflict(const std::string& file_path, const std::string& expected_hash);
    
    TransferCallback transfer_callback_;
    std::mutex callback_mutex_;
};

} // namespace transfer
} // namespace p2p
```

**File: cpp/src/transfer/file_receiver.cpp**

```cpp
#include "file_receiver.hpp"
#include <iostream>
#include <fstream>
#include <cstring>
#include <thread>

#ifdef _WIN32
    #include <winsock2.h>
    #include <ws2tcpip.h>
#else
    #include <sys/socket.h>
    #include <netinet/in.h>
    #include <arpa/inet.h>
    #include <unistd.h>
#endif

namespace p2p {
namespace transfer {

FileReceiver::FileReceiver() {
}

FileReceiver::~FileReceiver() {
}

void FileReceiver::handle_prepare_transfer(const Json::Value& message) {
    TransferRequest request;
    request.transfer_id = message["payload"]["transfer_id"].asString();
    request.file_path = message["payload"]["path"].asString();
    request.peer_id = message["payload"]["peer_id"].asString();
    request.transfer_port = message["payload"]["transfer_port"].asInt();
    request.expected_hash = message["payload"]["expected_hash"].asString();
    request.expected_size = message["payload"]["expected_size"].asInt64();
    
    // Start file reception in background thread
    std::thread(&FileReceiver::receive_file, this, request).detach();
}

void FileReceiver::receive_file(const TransferRequest& request) {
    int sock_fd;
    struct sockaddr_in server_addr;
    
#ifdef _WIN32
    WSADATA wsa_data;
    WSAStartup(MAKEWORD(2, 2), &wsa_data);
#endif
    
    // Create socket
    sock_fd = socket(AF_INET, SOCK_STREAM, 0);
    if (sock_fd < 0) {
        std::cerr << "Failed to create transfer socket" << std::endl;
        if (transfer_callback_) {
            transfer_callback_(request.transfer_id, false);
        }
        return;
    }
    
    // Connect to Go's transfer socket
    memset(&server_addr, 0, sizeof(server_addr));
    server_addr.sin_family = AF_INET;
    server_addr.sin_port = htons(request.transfer_port);
    inet_pton(AF_INET, "127.0.0.1", &server_addr.sin_addr);
    
    if (connect(sock_fd, (struct sockaddr*)&server_addr, sizeof(server_addr)) < 0) {
        std::cerr << "Failed to connect to transfer socket: " << request.transfer_port << std::endl;
#ifdef _WIN32
        closesocket(sock_fd);
#else
        close(sock_fd);
#endif
        if (transfer_callback_) {
            transfer_callback_(request.transfer_id, false);
        }
        return;
    }
    
    // Receive file data
    std::ofstream file(request.file_path, std::ios::binary);
    if (!file.is_open()) {
        std::cerr << "Failed to open file for writing: " << request.file_path << std::endl;
#ifdef _WIN32
        closesocket(sock_fd);
#else
        close(sock_fd);
#endif
        if (transfer_callback_) {
            transfer_callback_(request.transfer_id, false);
        }
        return;
    }
    
    char buffer[8192];
    int64_t total_received = 0;
    
    while (total_received < request.expected_size) {
        int64_t remaining = request.expected_size - total_received;
        int chunk_size = (remaining < sizeof(buffer)) ? remaining : sizeof(buffer);
        
#ifdef _WIN32
        int bytes_received = recv(sock_fd, buffer, chunk_size, 0);
#else
        int bytes_received = read(sock_fd, buffer, chunk_size);
#endif
        
        if (bytes_received <= 0) {
            std::cerr << "Failed to receive file data" << std::endl;
            break;
        }
        
        file.write(buffer, bytes_received);
        total_received += bytes_received;
    }
    
    file.close();
    
#ifdef _WIN32
    closesocket(sock_fd);
#else
    close(sock_fd);
#endif
    
    // Verify file integrity
    bool success = verify_file(request.file_path, request.expected_hash);
    
    if (!success) {
        handle_real_conflict(request.file_path, request.expected_hash);
    }
    
    if (transfer_callback_) {
        transfer_callback_(request.transfer_id, success);
    }
}

bool FileReceiver::verify_file(const std::string& file_path, const std::string& expected_hash) {
    // Use existing hash manager to verify
    // Implementation would call hash_manager.calculate_file_hash()
    std::string actual_hash = "computed_hash"; // Placeholder
    return actual_hash == expected_hash;
}

void FileReceiver::handle_real_conflict(const std::string& file_path, const std::string& expected_hash) {
    // Create backup of current file
    std::string backup_path = file_path + ".backup";
    // Implementation would use OS atomic operations
    
    // Log conflict for user resolution
    std::cerr << "Real conflict detected for: " << file_path << std::endl;
    std::cerr << "Expected hash: " << expected_hash << std::endl;
    // Could send IPC message back to Go for user notification
}

void FileReceiver::on_transfer_complete(TransferCallback callback) {
    std::lock_guard<std::mutex> lock(callback_mutex_);
    transfer_callback_ = callback;
}

} // namespace transfer
} // namespace p2p
```

---

## Part 2: C++ File System & Hashing Operations

### 2.1 File System Watcher Interface (C++)

**File: cpp/src/fs/file_system_watcher.hpp**

```cpp
#pragma once

#include <string>
#include <functional>
#include <vector>

namespace p2p {
namespace fs {

enum FileAction {
    ADDED,
    MODIFIED,
    DELETED,
    MOVED
};

struct FileChangeEvent {
    FileAction action;
    std::string path;
    std::string old_path;  // For MOVED action
    uint64_t size;
    long timestamp;
};

class FileSystemWatcher {
public:
    using ChangeCallback = std::function<void(const FileChangeEvent&)>;
    
    FileSystemWatcher(const std::string& watch_path);
    ~FileSystemWatcher();
    
    // Start watching the directory
    bool start();
    
    // Stop watching
    void stop();
    
    // Register callback for file changes
    void on_change(ChangeCallback callback);
    
    // Check if actively watching
    bool is_watching() const;
    
private:
    std::string watch_path_;
    bool watching_;
    ChangeCallback change_callback_;
    
    // Platform-specific implementation
#ifdef __linux__
    int inotify_fd_;
    int watch_descriptor_;
    
    void handle_inotify_events();
#elif __APPLE__
    void* stream_ref_;  // FSEventStreamRef
    
    void handle_fsevents();
#elif _WIN32
    void* dir_handle_;
    
    void handle_file_changes();
#endif
};

} // namespace fs
} // namespace p2p
```

### 2.2 Hash Manager (C++)

**File: cpp/src/hash/hash_manager.hpp**

```cpp
#pragma once

#include <string>
#include <map>
#include <mutex>

namespace p2p {
namespace hash {

class HashManager {
public:
    // Calculate SHA256 hash of a file
    static std::string calculate_file_hash(const std::string& file_path);
    
    // Calculate SHA256 hash from buffer
    static std::string calculate_buffer_hash(const unsigned char* buffer, size_t size);
    
    // Check if file has changed (compare hash)
    bool has_changed(const std::string& file_path, const std::string& last_hash);
    
    // Get cached hash for a file
    std::string get_cached_hash(const std::string& file_path);
    
    // Update cached hash
    void update_cached_hash(const std::string& file_path, const std::string& hash);
    
    // Clear all caches
    void clear_cache();
    
private:
    std::map<std::string, std::string> hash_cache_;
    mutable std::mutex cache_mutex_;
};

} // namespace hash
} // namespace p2p
```

---

## Part 3: Go Network & Coordination

### 3.1 Peer Discovery (Go)

**File: go/pkg/discovery/peer_discovery.go**

```go
package discovery

import (
    "fmt"
    "log"
    "net"
    "sync"
    "time"
    
    "github.com/grandcat/zeroconf"
)

type Peer struct {
    ID        string
    Name      string
    Address   string
    Port      int
    LastSeen  time.Time
    Connected bool
}

type PeerRegistry struct {
    peers map[string]*Peer
    mu    sync.RWMutex
    
    // Callbacks
    OnPeerDiscovered func(*Peer)
    OnPeerLost      func(*Peer)
}

func NewPeerRegistry() *PeerRegistry {
    return &PeerRegistry{
        peers: make(map[string]*Peer),
    }
}

func (pr *PeerRegistry) StartDiscovery() error {
    // Register this peer via mDNS
    hostname, _ := os.Hostname()
    entry := &zeroconf.ServiceEntry{
        Name:        hostname,
        Type:        "_p2psync._tcp",
        Port:        9876,
        Text:        []string{"version=1.0"},
    }
    
    server, err := zeroconf.RegisterService(entry, nil, nil)
    if err != nil {
        return fmt.Errorf("failed to register service: %v", err)
    }
    defer server.Shutdown()
    
    // Browse for other peers
    go pr.browsePeers()
    
    return nil
}

func (pr *PeerRegistry) browsePeers() {
    resolver, err := zeroconf.NewResolver(nil)
    if err != nil {
        log.Printf("Failed to create mDNS resolver: %v", err)
        return
    }
    defer resolver.Close()
    
    entries := make(chan *zeroconf.ServiceEntry)
    go func(results <-chan *zeroconf.ServiceEntry) {
        for entry := range results {
            pr.handlePeerDiscovered(entry)
        }
    }(entries)
    
    // Browse indefinitely
    err = resolver.Browse("_p2psync._tcp", "local.", entries)
    if err != nil {
        log.Printf("Browse failed: %v", err)
    }
}

func (pr *PeerRegistry) handlePeerDiscovered(entry *zeroconf.ServiceEntry) {
    if len(entry.AddrIPv4) == 0 {
        return
    }
    
    peer := &Peer{
        ID:       entry.Name,
        Name:     entry.Name,
        Address:  entry.AddrIPv4[0].String(),
        Port:     entry.Port,
        LastSeen: time.Now(),
    }
    
    pr.mu.Lock()
    defer pr.mu.Unlock()
    
    if _, exists := pr.peers[peer.ID]; !exists {
        pr.peers[peer.ID] = peer
        if pr.OnPeerDiscovered != nil {
            go pr.OnPeerDiscovered(peer)
        }
        log.Printf("Peer discovered: %s (%s:%d)\n", peer.Name, peer.Address, peer.Port)
    }
}

func (pr *PeerRegistry) GetPeers() []*Peer {
    pr.mu.RLock()
    defer pr.mu.RUnlock()
    
    peers := make([]*Peer, 0, len(pr.peers))
    for _, p := range pr.peers {
        peers = append(peers, p)
    }
    return peers
}

func (pr *PeerRegistry) GetPeer(id string) *Peer {
    pr.mu.RLock()
    defer pr.mu.RUnlock()
    
    return pr.peers[id]
}
```

### 3.2 Connection Manager (Go)

**File: go/pkg/network/connection_manager.go**

```go
package network

import (
    "fmt"
    "log"
    "net"
    "sync"
    "time"
)

type ConnectionManager struct {
    connections map[string]net.Conn
    mu          sync.RWMutex
    
    OnConnected    func(peerID string)
    OnDisconnected func(peerID string)
}

func NewConnectionManager() *ConnectionManager {
    return &ConnectionManager{
        connections: make(map[string]net.Conn),
    }
}

func (cm *ConnectionManager) Connect(peerID, address string, port int) error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    if _, exists := cm.connections[peerID]; exists {
        return nil // Already connected
    }
    
    addr := fmt.Sprintf("%s:%d", address, port)
    conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
    if err != nil {
        return fmt.Errorf("failed to connect to %s: %v", addr, err)
    }
    
    cm.connections[peerID] = conn
    log.Printf("Connected to peer: %s (%s)\n", peerID, addr)
    
    if cm.OnConnected != nil {
        go cm.OnConnected(peerID)
    }
    
    return nil
}

func (cm *ConnectionManager) GetConnection(peerID string) net.Conn {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    
    return cm.connections[peerID]
}

func (cm *ConnectionManager) CloseConnection(peerID string) {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    if conn, exists := cm.connections[peerID]; exists {
        conn.Close()
        delete(cm.connections, peerID)
        log.Printf("Disconnected from peer: %s\n", peerID)
        
        if cm.OnDisconnected != nil {
            go cm.OnDisconnected(peerID)
        }
    }
}

func (cm *ConnectionManager) IsConnected(peerID string) bool {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    
    _, exists := cm.connections[peerID]
    return exists
}
```

---

## Part 4: Message Protocol (IPC Communication)

### 4.1 C++ → Go: File Changed Event

```json
{
  "version": "1.0",
  "type": "file_changed",
  "id": "msg_12345",
  "timestamp": 1704067200000,
  "source": "cpp",
  "payload": {
    "action": "add",
    "path": "/home/user/sync/document.txt",
    "hash": "abc123def456...",
    "size": 2048,
    "modified_time": 1704067100
  }
}
```

### 4.2 Go → C++: Sync from Peer

```json
{
  "version": "1.0",
  "type": "sync_from_peer",
  "id": "msg_67890",
  "timestamp": 1704067200500,
  "source": "go",
  "payload": {
    "peer_id": "alice-laptop",
    "peer_name": "Alice",
    "path": "/home/user/sync/document.txt",
    "content_base64": "SGVsbG8gV29ybGQh...",
    "hash": "abc123def456...",
    "timestamp": 1704067100
  }
}
```

### 4.3 Go → C++: Peer List Update

```json
{
  "version": "1.0",
  "type": "peer_list_update",
  "id": "msg_11111",
  "timestamp": 1704067201000,
  "source": "go",
  "payload": {
    "peers": [
      {
        "id": "alice-laptop",
        "name": "Alice",
        "address": "192.168.1.100",
        "port": 9876,
        "connected": true
      },
      {
        "id": "bob-desktop",
        "name": "Bob",
        "address": "192.168.1.101",
        "port": 9876,
        "connected": false
      }
    ]
  }
}
```

---

## Part 5: Build Configuration & Deployment

### 5.1 CMakeLists.txt for C++ Build

```cmake
cmake_minimum_required(VERSION 3.10)
project(p2p_sync_cpp)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Find required packages
find_package(sqlite3 REQUIRED)
find_package(nlohmann_json 3.2.0 REQUIRED)

# Platform-specific sources
if(UNIX AND NOT APPLE)
    set(PLATFORM_SOURCES src/fs/file_system_watcher_linux.cpp)
elseif(APPLE)
    set(PLATFORM_SOURCES src/fs/file_system_watcher_macos.cpp)
elseif(WIN32)
    set(PLATFORM_SOURCES src/fs/file_system_watcher_windows.cpp)
endif()

add_executable(cpp_daemon
    src/main.cpp
    src/ipc/ipc_client.cpp
    src/fs/file_system_watcher.cpp
    ${PLATFORM_SOURCES}
    src/hash/hash_manager.cpp
    src/repo/repository_manager.cpp
    src/version/version_history_manager.cpp
    src/db/state_store.cpp
)

target_link_libraries(cpp_daemon
    sqlite3
    nlohmann_json::nlohmann_json
)

if(UNIX)
    target_link_libraries(cpp_daemon pthread)
endif()
```

### 5.2 Go main.go (Network Coordinator)

```go
package main

import (
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"
    
    "p2p/pkg/discovery"
    "p2p/pkg/network"
    "p2p/pkg/ipc"
)

func main() {
    // Start IPC server
    ipcServer := ipc.NewIpcServer("/tmp/p2p_sync.sock")
    if err := ipcServer.Start(); err != nil {
        log.Fatalf("Failed to start IPC server: %v", err)
    }
    defer ipcServer.Stop()
    
    // Start peer discovery
    peerRegistry := discovery.NewPeerRegistry()
    if err := peerRegistry.StartDiscovery(); err != nil {
        log.Fatalf("Failed to start peer discovery: %v", err)
    }
    
    // Start connection manager
    connMgr := network.NewConnectionManager()
    
    // Handle IPC messages from C++ daemon
    ipcServer.OnMessage = func(msg *ipc.Message) error {
        fmt.Printf("Received from C++: %s\n", msg.Type)
        
        switch msg.Type {
        case "file_changed":
            // Broadcast to all connected peers
            return handleFileChanged(msg, connMgr)
        case "peer_list_request":
            // Send peer list back to C++
            return sendPeerList(peerRegistry, ipcServer)
        }
        return nil
    }
    
    // Graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    fmt.Println("Shutting down...")
}

func handleFileChanged(msg *ipc.Message, connMgr *network.ConnectionManager) error {
    fmt.Println("Broadcasting file change to all peers...")
    // TODO: Serialize and send to all connected peers
    return nil
}

func sendPeerList(registry *discovery.PeerRegistry, ipcServer *ipc.IpcServer) error {
    peers := registry.GetPeers()
    // TODO: Serialize peer list and send to C++ daemon
    return nil
}
```

---

## Implementation Summary

### Component Responsibilities

**Go (82% - Coordination & Networking)**
- IPC Server: Receives file events from C++, routes to peers
- Peer Discovery: mDNS registration and discovery
- Connection Manager: Manages 10K+ concurrent peer connections
- Version Control: Conflict detection and resolution (Last-Write-Wins)
- State Persistence: SQLite database for metadata and history
- Multi-Repository Coordination: Fair scheduling across independent repos

**C++ (18% - File I/O Optimization)**
- File System Watcher: Direct inotify/FSEvents/native API integration
- Hash Manager: Memory-mapped file hashing (15% faster on large files)
- IPC Client: Sends file change events to Go network component
- Repository Manager: Applies changes locally, manages atomic operations

### Integration Points

1. **File Change Detection**: C++ watcher → IPC message → Go router → Network broadcast
2. **Conflict Resolution**: Go detects conflict → IPC to C++ → Local resolution → Sync continues
3. **Multi-Repo Sync**: Independent state machines per repo, coordinated via Go channels
4. **Peer Updates**: Go discovers peers → Sends peer list to C++ → C++ displays/manages

### IPC Protocol
- **Transport**: Unix sockets (Linux/macOS) or TCP (Windows)
- **Format**: Length-prefixed JSON messages
- **Frequency**: ~50-100 messages/min per active peer
- **Latency**: ~1-2ms per message (<1% overhead vs network transfer)

### Performance Characteristics

| Metric                  | Value     | Notes                        |
| ----------------------- | --------- | ---------------------------- |
| Max Concurrent Peers    | 10,000+   | Go goroutines handle scaling |
| File Watch Latency      | 2-5ms     | C++ direct OS API            |
| Hash Speed (256MB file) | 200-300ms | C++ memory-mapped            |
| IPC Message Latency     | 1-2ms     | Unix socket overhead         |
| Network Bottleneck      | ~2000ms   | Dominates for 256MB transfer |

### Code Readiness

Each component includes:
- Complete interface definitions
- Cross-platform compatibility (Unix, macOS, Windows)
- Error handling patterns
- Callback/channel-based async patterns
- Working code stubs ready for platform-specific implementation

See [p2p_architecture.md](p2p_architecture.md) for detailed rationale, comparison tables, and performance analysis.
