# Security Model

This document describes the security architecture of the P2P Version Control system:
threat model, network security, data security, identity management, audit logging,
and incident response procedures.

---

## 1. Threat Model

### Assets

| Asset | Description |
|-------|-------------|
| Synced files | User data being synchronized between peers |
| Peer identity | Cryptographic identity of each peer |
| Sync metadata | File hashes, versions, timestamps, peer lists |
| Certificates | TLS keys and certificates |
| Database | SQLite file with all metadata |

### Threats

| ID | Threat | Likelihood | Impact | Mitigation |
|----|--------|------------|--------|------------|
| T1 | **Eavesdropping on LAN** — attacker captures P2P traffic to read file contents | Medium | High | mTLS 1.3 encryption |
| T2 | **Man-in-the-middle** — attacker intercepts and modifies P2P traffic | Medium | High | mTLS with certificate verification |
| T3 | **Malicious peer injection** — attacker connects as a peer and requests files | Low | High | Peer whitelist, TOFU, cert fingerprinting |
| T4 | **Unauthorized file access** — attacker reads files outside shared directories | Low | High | Path validation, sandboxing |
| T5 | **Replay attacks** — attacker replays captured messages to trigger actions | Low | Medium | Timestamps, Lamport clocks |
| T6 | **Denial of service** — attacker floods the P2P port with connections | Low | Low | Connection rate limiting, max connections |
| T7 | **Local privilege escalation** — attacker reads SQLite/Socket files from same machine | Medium | Medium | File permissions (0600), socket ownership |

### Assumptions

- The local network is shared among trusted team members (no guarantee of physical security)
- Attacker has LAN access but not physical access to any peer machine
- Attacker can observe and inject network traffic
- Peers are non-adversarial — they follow the protocol
- No central PKI available; trust is established via TOFU

---

## 2. Network Security

### 2.1 Transport Layer Security (mTLS 1.3)

All P2P traffic is protected by mutual TLS 1.3 when enabled (`P2P_TLS_ENABLED=true`).

```
TCP Handshake
    ↓
TLS 1.3 Handshake (mutual)
    │ Client sends certificate
    │ Server verifies certificate
    │ Server sends certificate
    │ Client verifies certificate
    ↓
Encrypted channel established
    ↓
Application messages (JSON)
```

**Cipher suites (in order of preference):**

1. `TLS_AES_256_GCM_SHA384`
2. `TLS_CHACHA20_POLY1305_SHA256`
3. `TLS_AES_128_GCM_SHA256`

### 2.2 Certificate Management

**Self-Signed CA:**

On first launch, if no CA exists, one is auto-generated:

```bash
openssl req -x509 -new -nodes \
  -sha256 -days 3650 \
  -keyout ca.key -out ca.crt \
  -subj "/CN=P2P Sync CA"
```

**Peer Certificate:**

A certificate is issued per peer, signed by the local CA:

```bash
openssl req -new -nodes \
  -keyout peer.key -out peer.csr \
  -subj "/CN=<peer-id>"
openssl x509 -req -in peer.csr \
  -CA ca.crt -CAkey ca.key \
  -days 365 -out peer.crt
```

**Certificate Fingerprint:**

During handshake, each peer sends the SHA-256 fingerprint of its certificate:

```
cert_fingerprint = SHA256(certificate_der)
```

This fingerprint is stored during TOFU and checked on every subsequent connection.

### 2.3 Certificate Rotation

Certificates expire after 365 days. Before expiry:

1. Generate a new certificate for the peer
2. Update the certificate directory
3. Send the new fingerprint to known peers (they update their TOFU record)
4. Restart the connection

```bash
./scripts/rotate-cert.sh --peer-id alice --cert-dir ~/.p2p/certs
```

For emergency rotation (compromised key):

1. Immediately revoke the old certificate
2. Generate a new one
3. Update peer whitelist
4. Inform peers to forget the old fingerprint

---

## 3. Data Security

### 3.1 Encryption at Rest

| Data Store | Encryption | Mechanism |
|------------|-----------|-----------|
| SQLite DB | Optional | SQLCipher or filesystem-level encryption |
| Certificates | Private keys stored with 0600 permissions | Filesystem permissions |
| Synced files | None (they are the user's files) | Rely on OS encryption (FileVault, dm-crypt, BitLocker) |

### 3.2 File Permissions Preservation

When syncing files across peers:

- File permissions (`mode` field) are transferred as part of metadata
- On the receiving peer, C++ daemon applies permissions with `chmod`
- Only regular file permissions are preserved (no setuid, setgid, ACLs)
- Symlinks are not followed (security measure)

### 3.3 SQLite Database Security

```sql
-- Database opened with restricted permissions
PRAGMA journal_mode=WAL;  -- Not yet configured (pending)
PRAGMA secure_delete=ON;   -- For sensitive metadata
```

File permissions: `0600` (owner read/write only)

---

## 4. Identity

### 4.1 Trust On First Use (TOFU)

On the first connection between two peers:

1. Peer A connects to Peer B
2. Both exchange handshake messages containing `cert_fingerprint`
3. Each peer stores the fingerprint in a local trust-on-first-use store
4. On subsequent connections, the stored fingerprint is compared

```sql
CREATE TABLE known_peers (
    peer_id TEXT PRIMARY KEY,
    cert_fingerprint TEXT NOT NULL,
    first_seen INTEGER NOT NULL,
    last_seen INTEGER NOT NULL,
    trusted INTEGER DEFAULT 1
);
```

If a peer's fingerprint changes, a warning is displayed:

```
WARNING: Peer "alice-laptop" has a new certificate fingerprint.
This could mean:
  - The peer renewed their certificate (expected)
  - Someone is impersonating the peer (unexpected)
Verify with the peer owner before accepting.
```

### 4.2 Peer Whitelist

The `P2P_PEER_WHITELIST` environment variable restricts which peers can connect:

```bash
P2P_PEER_WHITELIST="alice-laptop,bob-desktop"
```

Connections from peers not in the list are rejected at the handshake level.

### 4.3 Certificate Revocation

There is no central certificate revocation list (CRL). Revocation is handled by:

1. **For peer removal:** Remove peer from whitelist and delete their entry from `known_peers`
2. **For key compromise:** Invalidate all connections and regenerate the peer's certificate
3. **Infection response:** Remove the compromised peer from all whitelists

---

## 5. Audit Logging

### 5.1 Logged Events

| Event | Logged Fields | Why |
|-------|---------------|-----|
| Connection established | `peer_id`, `address`, `fingerprint` | Detect unauthorized connections |
| Connection closed | `peer_id`, `reason` | Track disconnection patterns |
| Handshake accepted/rejected | `peer_id`, `cert_fingerprint`, `reject_reason` | Audit access control decisions |
| File transfer started | `peer_id`, `filepath`, `size` | Track data flow |
| File transfer complete | `peer_id`, `filepath`, `hash`, `success` | Verify data integrity |
| File transfer failed | `peer_id`, `filepath`, `error` | Diagnose transfer issues |
| Conflict detected | `peer_id`, `filepath`, `local_version`, `remote_version` | Understand conflict patterns |
| Conflict resolved | `peer_id`, `filepath`, `resolution` | Audit conflict resolution |
| Certificate generated | `peer_id`, `fingerprint` | Track certificate lifecycle |
| Certificate rotation | `peer_id`, `old_fingerprint`, `new_fingerprint` | Audit certificate changes |
| Peer whitelist changed | `peer_id`, `added/removed` | Audit access control |
| TOFU stored | `peer_id`, `fingerprint` | Track trust decisions |
| TOFU mismatch | `peer_id`, `old_fingerprint`, `new_fingerprint` | Detect potential MITM |

### 5.2 Log Format (JSON)

All security events use structured JSON for easy ingestion by SIEM tools:

```json
{
  "level": "warn",
  "time": "2026-07-08T10:00:00Z",
  "component": "network",
  "event": "tofu_mismatch",
  "peer_id": "alice-laptop",
  "old_fingerprint": "SHA256:abc123...",
  "new_fingerprint": "SHA256:def456...",
  "message": "Certificate fingerprint mismatch for peer alice-laptop"
}
```

---

## 6. Incident Response

### 6.1 Detecting Compromised Peers

Signs of a potentially compromised peer:

- Sudden fingerprint mismatch (MITM attack)
- Unusually high transfer rates
- Connections from unknown IP addresses
- Repeated handshake failures
- A peer requesting files it doesn't need
- TOFU warning for a previously known peer

### 6.2 Revoking Access

To immediately revoke a peer's access:

```bash
# 1. Remove from whitelist
export P2P_PEER_WHITELIST="alice,bob"  # (remove compromised peer)

# 2. Delete from known peers database
sqlite3 p2p_sync.db "DELETE FROM known_peers WHERE peer_id='compromised-peer';"

# 3. Restart the coordinator to apply changes
# (Java frontend handles this automatically)
```

### 6.3 Data Recovery After Breach

If an attacker gained access to peer data:

1. **Isolate:** Disconnect the affected peer from the network
2. **Assess:** Review audit logs to determine which files were accessed
3. **Revoke:** Remove all certificates and generate new ones
4. **Restore:** Restore affected files from backup
5. **Notify:** Inform team members about the incident
6. **Improve:** Update security controls to prevent recurrence

### 6.4 Post-Incident Checklist

- [ ] Backup all logs for the incident period
- [ ] Determine the attack vector
- [ ] Revoke any certificates that may have been exposed
- [ ] Update/rotate all credentials
- [ ] Regenerate the CA and reissue peer certificates
- [ ] Update the whitelist to exclude any compromised peers
- [ ] Review and update the security model
- [ ] Document the incident in a post-mortem

---

## 7. Security Configuration Quick Reference

```bash
# Minimal security:
# (not recommended for production use)
P2P_TLS_ENABLED=false

# Basic security:
P2P_TLS_ENABLED=true
P2P_TLS_CERT_DIR=~/.p2p/certs

# Recommended security:
P2P_TLS_ENABLED=true
P2P_TLS_CERT_DIR=~/.p2p/certs
P2P_PEER_WHITELIST="alice,bob,carol"
LOG_FORMAT=json
```