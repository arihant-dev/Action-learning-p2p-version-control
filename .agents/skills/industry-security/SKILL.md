---
name: industry-security
description: TLS encryption, peer authentication, encryption at rest, and security hardening.
---

# Security Agent

**Role:** Implement zero-trust security across all communication layers
**Branch:** `ws/security` (branched from `master`)
**Merge target:** `industry-shipping`
**Phase:** 1 (design) + 2 (implementation, depends on config system)

## Work Items (in priority order)

1. **TLS 1.3 for P2P TCP**
   - Generate self-signed CA and peer certificates at first launch
   - Wrap P2P TCP connections in mutual TLS (mTLS)
   - Verify peer certificate against CA during handshake
   - Certificate rotation mechanism (configurable)
   - Store certificates in configurable directory

2. **TLS for IPC**
   - Add optional TLS wrapping for TCP fallback IPC (`:9999`)
   - Unix sockets remain as-is (filesystem permissions are the security boundary)
   - Auto-detect: use TLS if TCP, plain Unix socket if available

3. **Peer Identity Verification**
   - Replace simple peer ID strings with certificate fingerprint-based identity
   - Verify mDNS-discovered peers match their announced identity
   - Reject connections from unknown/untrusted peers (configurable whitelist)

4. **Encryption at Rest (SQLite)**
   - Migrate from plain SQLite to SQLCipher or use application-layer encryption
   - Encrypt sensitive fields: peer IDs, file paths (if configured)
   - Master key from config file or keychain

5. **Security Audit Logging**
   - Log all security events: connections, disconnections, auth failures, certificate expirations
   - Separate log file or structured log with `security` component tag
   - Include timestamps, peer IDs, IP addresses, event types

6. **Rate Limiting & Hardening**
   - Rate limit incoming P2P connection attempts (per IP)
   - Input validation: maximum message size, field length limits, type checking
   - Reject malformed messages with descriptive error
   - Connection timeout: complete handshake within 10 seconds or drop

## Relevant Files
- `src/backend/go/pkg/network/connection_manager.go` — P2P TCP logic
- `src/backend/go/pkg/discovery/peer_discovery.go` — mDNS discovery
- `src/backend/go/pkg/ipc/ipc_server.go` — IPC server
- `src/backend/go/main.go` — Startup wiring
- `src/backend/go/pkg/storage/sqlite/db.go` — Database initialization
- `src/backend/go/pkg/protocol/messages.go` — Message types (may need TLS config messages)

## Verification
- `cd src/backend/go && go build ./...` — must compile
- `cd src/backend/go && go test ./... -count=1` — all tests pass
- Two peers with valid certificates should handshake successfully
- Peer with invalid certificate should be rejected
- SQLite DB should not contain readable plaintext after encryption
