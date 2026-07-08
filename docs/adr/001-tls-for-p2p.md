# ADR-001: TLS Mutual Authentication for P2P Communication

**Status:** Accepted
**Date:** 2026-07-08
**Deciders:** Architecture Team

---

## Context

P2P communication between peers happens over TCP (port 9876). Without encryption,
an attacker on the same local network can eavesdrop on all synced file contents
and metadata ("T1: Eavesdropping on LAN" per security model). Without authentication,
an attacker can impersonate a trusted peer ("T2: Man-in-the-middle").

The system needs:
- Confidentiality: file contents should not be readable on the wire
- Peer identity verification: each peer must prove who they are
- Forward secrecy: past sessions should not be compromised by future key leaks

## Decision

We will use **mutual TLS 1.3 (mTLS)** for all P2P communication, with a
**self-signed CA** auto-generated on first launch.

### Key Design Choices

| Choice | Rationale |
|--------|-----------|
| TLS 1.3 (not 1.2) | Forward secrecy by default, fewer cipher suites, faster handshake |
| Self-signed CA | No dependency on external PKI; suitable for LAN-only deployment |
| TOFU identity binding | First-seen fingerprint is trusted; mismatches alert the user |
| Certificate fingerprint in handshake | Lightweight identity assertion without full cert exchange |
| Optional (env var toggle) | Development environments may disable TLS for debugging |

### Certificate Flow

1. On first launch, if no CA exists, generate a self-signed CA (valid 10 years)
2. Each peer generates a certificate signed by its CA (valid 1 year)
3. The SHA-256 fingerprint of the certificate is exchanged during handshake
4. The fingerprint is stored in the `known_peers` table (TOFU)
5. On subsequent connections, the fingerprint is verified against the stored value

### TLS Cipher Suites (in preference order)

1. `TLS_AES_256_GCM_SHA384`
2. `TLS_CHACHA20_POLY1305_SHA256`
3. `TLS_AES_128_GCM_SHA256`

## Consequences

**Positive:**
- All P2P traffic is encrypted and authenticated
- Peer identity is verified via certificate fingerprint
- No external PKI required; setup is fully automatic
- Forward secrecy protects past sessions

**Negative:**
- Increased CPU usage for encryption (negligible for metadata, measurable for large files)
- TLS adds ~1-2 round trips to connection establishment
- Certificate management is more complex (rotation, storage)
- TOFU is vulnerable on the very first connection (no prior trust anchor)

**Mitigations:**
- Peer whitelist (`P2P_PEER_WHITELIST`) provides an out-of-band trust anchor
- Certificate rotation scripts are provided
- Audit logging traces all certificate-related events
- TOFU mismatches are logged and alerted

## Status

Accepted. Implementation: Go uses `crypto/tls` for the TCP listener and dialer.
Certificate generation is handled by a helper package.

## References

- [Security Model](../security/security-model.md)
- P2P Protocol Specification §8: Security