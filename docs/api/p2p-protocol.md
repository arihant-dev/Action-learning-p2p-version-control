# P2P Protocol Specification

**Version:** 1.0
**Transport:** TCP
**Default Port:** 9876
**Last Updated:** 2026-07-08

---

## 1. Overview

The P2P protocol governs communication between Go coordinator instances across different
peers on the same local network. It uses framed JSON messages over TCP for the **control
plane** and raw TCP streams on ephemeral ports for the **data plane** (file content).

### Protocol Stack

```
┌──────────────────────────────────────┐
│         Message Layer (JSON)         │
├──────────────────────────────────────┤
│        Framing Layer (4-byte len)    │
├──────────────────────────────────────┤
│          TCP Transport Layer         │
├──────────────────────────────────────┤
│  TLS 1.3 (optional, mTLS for P2P)   │
└──────────────────────────────────────┘
```

### Port Usage

| Port    | Purpose          | Protocol | Encryption |
|---------|------------------|----------|------------|
| 9876    | P2P control      | TCP+JSON | TLS 1.3    |
| ephemeral | File transfers | TCP raw  | Via TLS    |
| 8080    | HTTP health      | HTTP     | None       |

---

## 2. Connection Establishment

### 2.1 TCP Connection

Peers connect to each other on TCP port 9876. The initiator connects to the listener.

```plaintext
Peer A ───SYN───▶ Peer B
Peer A ◀──SYN-ACK─ Peer B
Peer A ───ACK───▶ Peer B
Peer A ───HANDSHAKE───▶ Peer B
Peer A ◀──HANDSHAKE ACK─ Peer B
```

### 2.2 Handshake

Immediately after TCP establishment, both peers exchange a handshake message:

```json
{
  "version": "1.0",
  "type": "handshake",
  "id": "msg_1712345678",
  "timestamp": 1712345678000,
  "source": "go",
  "payload": {
    "peer_id": "alice-mbp",
    "display_name": "Alice",
    "protocol_version": "1.0",
    "capabilities": ["file_sync", "delta_sync", "mTLS"],
    "cert_fingerprint": "SHA256:abc123def456...",
    "listen_port": 9876
  }
}
```

**Handshake fields:**

| Field              | Required | Description                            |
| ------------------ | -------- | -------------------------------------- |
| `peer_id`          | yes      | Unique peer identifier                 |
| `display_name`     | no       | Human-readable name                    |
| `protocol_version` | yes      | Protocol version for negotiation       |
| `capabilities`     | no       | Array of supported features            |
| `cert_fingerprint` | yes      | SHA-256 of peer TLS certificate        |
| `listen_port`      | yes      | Port the peer listens on               |

Upon receiving a handshake, the receiver **must** respond:

```json
{
  "version": "1.0",
  "type": "handshake",
  "id": "msg_1712345680",
  "timestamp": 1712345680000,
  "source": "go",
  "payload": {
    "peer_id": "bob-desktop",
    "display_name": "Bob",
    "protocol_version": "1.0",
    "capabilities": ["file_sync"],
    "cert_fingerprint": "SHA256:def456abc789...",
    "listen_port": 9876,
    "accepted": true,
    "reject_reason": null
  }
}
```

If the peer rejects the handshake (e.g. protocol mismatch, not whitelisted):

```json
{
  "version": "1.0",
  "type": "handshake",
  "payload": {
    "accepted": false,
    "reject_reason": "protocol_version_mismatch",
    "accepted_version": "1.0"
  }
}
```

### 2.3 Protocol Version Negotiation

- Both sides advertise their `protocol_version`
- If versions differ, the **higher** version is preferred if both peers support it
- Followed by capability intersection
- If no compatible version exists, connection is rejected

---

## 3. Message Framing

### 3.1 Frame Format

```
┌───────────────────────────────┐
│  4-byte Length (big-endian)   │  ← uint32, max 1MB
├───────────────────────────────┤
│  UTF-8 JSON Message Body      │  ← up to length bytes
└───────────────────────────────┘
```

### 3.2 Common Message Envelope

Every message shares this envelope:

```json
{
  "version": "1.0",
  "type": "<message_type>",
  "id": "msg_<timestamp>",
  "timestamp": 1704067200000,
  "source": "go",
  "payload": {}
}
```

| Field       | Type    | Description                        |
|-------------|---------|------------------------------------|
| `version`   | string  | Protocol version                    |
| `type`      | string  | Message type (see §4)               |
| `id`        | string  | Unique message identifier           |
| `timestamp` | integer | Unix epoch milliseconds              |
| `source`    | string  | Originating component (`go`/`cpp`/`java`) |
| `payload`   | object  | Type-specific payload                |

---

## 4. Message Types

### 4.1 Heartbeat

Sent every 5 seconds (configurable via `HEARTBEAT_INTERVAL`).

```json
// Ping
{
  "version": "1.0",
  "type": "ping",
  "id": "msg_1712345700",
  "timestamp": 1712345700000,
  "source": "go",
  "payload": {}
}

// Pong response
{
  "version": "1.0",
  "type": "pong",
  "id": "msg_1712345700",
  "timestamp": 1712345700100,
  "source": "go",
  "payload": {}
}
```

| Behavior               | Value |
| ---------------------- | ----- |
| Heartbeat interval     | 5s    |
| Timeout (missed pings) | 15s   |
| Reconnect backoff      | 1s→60s (exponential) |

After 3 missed heartbeats (15s timeout), the connection is considered dead
and the auto-reconnect loop begins.

### 4.2 File Metadata Update

Broadcast when a file changes on a peer:

```json
{
  "type": "file_metadata_update",
  "payload": {
    "repo_id": "repo_abc123",
    "filepath": "docs/design.md",
    "hash": "a1b2c3d4e5f6...",
    "size": 2048,
    "version": 3,
    "lamport_clock": 5,
    "vector_clock": {"alice-mac": 3, "bob-desktop": 2},
    "timestamp": 1704067200,
    "mode": 644
  }
}
```

### 4.3 File Request

Request a file from a peer:

```json
{
  "type": "file_request",
  "payload": {
    "filepath": "docs/design.md",
    "repo_id": "repo_abc123",
    "request_id": "req-001"
  }
}
```

### 4.4 File Response

Respond with transfer port or error:

```json
{
  "type": "file_response",
  "payload": {
    "request_id": "req-001",
    "accepted": true,
    "transfer_port": 43210,
    "expected_hash": "a1b2c3d4e5f6...",
    "expected_size": 1048576
  }
}
```

On error:

```json
{
  "type": "file_response",
  "payload": {
    "request_id": "req-001",
    "accepted": false,
    "error": "file_not_found"
  }
}
```

### 4.5 Error Message

```json
{
  "type": "error",
  "payload": {
    "code": "INVALID_MESSAGE",
    "description": "Unrecognized message type: foobar",
    "original_message_id": "msg_12345"
  }
}
```

**Error codes:**

| Code                       | Description                        |
| -------------------------- | ------------------------------ |
| `INTERNAL_ERROR`           | Unexpected server error        |
| `INVALID_MESSAGE`        | Malformed or unknown message   |
| `PROTOCOL_MISMATCH`      | Incompatible protocol version  |
| `PEER_NOT_WHITELISTED`   | Peer not allowed               |
| `FILE_NOT_FOUND`         | Requested file not available   |
| `TRANSFER_FAILED`        | File transfer encountered error |
| `RATE_LIMITED`           | Too many requests              |

### 4.6 Disconnect / Goodbye

```json
{
  "type": "goodbye",
  "payload": {
    "reason": "shutdown"
  }
}
```

---

## 5. File Transfer Protocol

### 5.1 Control Plane vs Data Plane

File transfers use a **split-plane** architecture:

```
Control Plane (TCP :9876)     Data Plane (TCP ephemeral)
┌─────────────────┐                   ┌─────────────────┐
│ Go Coordinator │───metadata──▶  │ Go Coordinator │
│ (Peer A)       │◀──response──  │ (Peer A)       │
└─────────────────┘           └────────┬────────┘
                        ┌────────────────▼───────┐
                        │  C++ Daemon (disk)  │
                        └────────────────────────┘
```

### 5.2 Transfer Sequence

```
Requesting Peer (Downloader)         Responding Peer (Uploader)
        │                                │
        │──── file_request ──────────────▶│
        │                                │
        │◀──── file_response (port) ─────│
        │                                │
        │  ╔═══════════════════════╗     │
        │  ║  Data Plane: TCP :N   ║     │
        │  ║  Raw file content     ║     │
        │  ╚═══════════════════════╝     │
        │                                │
        │──── file_transfer_ack ────────▶│
```

### 5.3 Proxy Pattern

The Go coordinator acts as a **TCP proxy**:

```
Remote Peer (TCP)        Go Coordinator        C++ Daemon (localhost)
    │                         │                       │
    │     send file data      │                       │
    │────────────────────────▶│     ipc: prepare      │
    │                         │──────────────────────▶│
    │                         │     accept local conn  │
    │                         │◀──────────────────────│
    │                         │                       │
    │                         │── io.Copy (proxy) ──▶│
    │                         │                       │
    │                         │  ipc: transfer_complete│
    │                         │──────────────────────▶│
```

1. Go creates a **listening socket** on a random ephemeral port
2. Go sends `prepare_file_transfer` IPC to C++ with the port
3. C++ connects to the local port
4. Go sends `file_response` P2P message to remote peer with the same port
5. Remote peer connects and streams data through Go → C++ → disk
6. On completion, `file_transfer_complete` IPC message is sent

### 5.4 Transfer Constraints

| Parameter            | Value           |
| -------------------- | --------------- |
| Max concurrent transfers | 4            |
| Read buffer          | 4096 bytes     |
| Max message size     | 1 MB           |
| Transfer timeout     | 300s (5 min)   |

---

## 6. Heartbeat and Keep-Alive

| Parameter                   | Default | Description                     |
| --------------------------- | ------- | ------------------------------- |
| `HEARTBEAT_INTERVAL`       | 5s      | Interval between ping messages   |
| `HEARTBEAT_TIMEOUT`        | 15s     | Timeout before disconnect        |
| `RECONNECT_BASE_DELAY`     | 1s      | Initial reconnect delay          |
| `RECONNECT_MAX_DELAY`      | 60s     | Maximum reconnect delay          |

### Auto-Reconnect

When a connection drops:

1. Wait `RECONNECT_BASE_DELAY` (1s)
2. Attempt reconnect
3. On failure, double the delay (2s → 4s → 8s → ... → 60s max)
4. On success, reset delay to `RECONNECT_BASE_DELAY`

---

## 7. Error Handling & Recovery

### 7.1 Graceful Disconnect

A peer that shuts down normally sends a `goodbye` message. Peers immediately
remove it from their peer list without reconnect attempts.

### 7.2 Unexpected Disconnect

1. Heartbeat timeout (15s without pong)
2. Mark connection as dead
3. Remove peer from connected list
4. Start auto-reconnect loop
5. If reconnected: re-handshake, resume sync
6. All in-progress transfers are cancelled

### 7.3 Transfer Errors

| Error             | Recovery                          |
| ----------------- | --------------------------------- |
| `FILE_NOT_FOUND` | Notify requesting peer, no retry   |
| `TRANSFER_FAILED`| Retry up to 3 times, then cancel  |
| `DISK_FULL`      | Pause sync, notify user            |
| `PERMISSION_DENIED` | Log error, skip file, continue |

---

## 8. Security

P2P communication supports mutual TLS (mTLS) 1.3. See [Security Model](../security/security-model.md)
for details.

### TLS Handshake Constraint

If TLS is enabled (`P2P_TLS_ENABLED=true`), the TCP connection is wrapped in TLS
**immediately after TCP handshake**, before any JSON messages are exchanged.

```
TCP Handshake
    ↓
TLS Handshake (mTLS 1.3)
    │ certificate verification
    │ fingerprint matching
    │ peer whitelist check
    ↓
JSON Handshake (application-level)
    ↓
Normal messaging
```

---

## 9. Protocol Versioning Policy

- Versions follow `MAJOR.MINOR` (e.g. `1.0`, `1.1`, `2.0`)
- **MAJOR** version bumps: breaking changes (wire format, handshake, required fields)
- **MINOR** version bumps: additive changes (new message types, optional fields)
- Peers negotiate the highest mutually supported version
- Unknown message types are silently ignored (for forward compatibility)
- New optional fields must be ignorable by older peers

---

## Appendix: Example Session

```
Alice connects to Bob via mDNS discovery

TCP: Alice:9876  →  Bob:9876  (connect)

Alice → Bob: HANDSHAKE {peer_id:"alice", protocol_version:"1.0"}
Bob → Alice: HANDSHAKE {peer_id:"bob", protocol_version:"1.0", accepted:true}

[Heartbeat every 5s]

Alice detects file change
Alice → Bob: FILE_METADATA_UPDATE {filepath:"doc.txt", hash:"abc", version:2}
Bob → Alice: FILE_REQUEST {filepath:"doc.txt"}
Alice → Bob: FILE_RESPONSE {transfer_port:40001, accepted:true}

[Data plane: Bob connects to Alice:40001, streams file]

Alice → Bob: ACK

[Bob's C++ receives, verifies hash, atomically applies]

Alice → Bob: PING
Bob → Alice: PONG

[... time passes ...]

Alice → Bob: GOODBYE {reason:"shutdown"}
TCP disconnect
```