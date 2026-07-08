# ADR-0001: TLS Mutual Authentication for P2P Connections

**Status:** Proposed
**Date:** 2026-07-08
**Author:** Roadmap Agent

## Context
P2P traffic is currently transmitted as plaintext over TCP. Any host on the same LAN can eavesdrop on file synchronization data, inject malicious file changes, or impersonate a peer. There is no mechanism for verifying the identity of connecting peers, making the system vulnerable to man-in-the-middle attacks and unauthorized data access.

## Decision
Adopt mutual TLS 1.3 (mTLS) for all P2P TCP connections with:

- A self-signed certificate authority (CA) generated automatically on first launch of the Go coordinator
- Each peer generates its own keypair signed by the local CA
- Peer identity is verified via the SHA-256 fingerprint of the peer's certificate
- Certificate fingerprints are exchanged out-of-band during initial peer pairing and stored in the SQLite database
- TLS 1.3 only (no fallback to older versions) for forward secrecy and reduced handshake latency

## Consequences
- **Positive:** Strong encryption for all P2P traffic; peer identity verification prevents spoofing; self-signed CA means no external PKI dependency
- **Negative:** Increased CPU cost for TLS handshake (~1-2ms per connection); more complex connection setup (certificate generation, fingerprint exchange); first-run experience must handle CA creation
- **Risks:** Self-signed CA means no revocation mechanism; if the CA private key is compromised, all peers must re-pair

## Alternatives Considered
- **Pre-shared keys (PSK):** Simpler to implement, lower CPU overhead, but less scalable — each peer pair needs a unique key, and key rotation is manual
- **WireGuard:** Excellent security and performance, but requires kernel module installation (Linux) or userspace implementation (cross-platform complexity), and adds a network interface dependency
- **Plaintext with HMAC:** No encryption, only integrity — still leaks file metadata and content sizes
