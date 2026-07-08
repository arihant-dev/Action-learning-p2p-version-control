# ADR-0006: Chunked File Transfer with Resume Support

**Status:** Proposed
**Date:** 2026-07-08
**Author:** Roadmap Agent

## Context
Files are currently transferred as a single continuous stream. If the connection is interrupted mid-transfer, the entire file must be retransmitted. There is no per-chunk checksum verification, meaning corruption in transit is not detected until the full file hash is computed at the end. Large files (100MB+) can take minutes and any interruption forces a full restart.

## Decision
Implement chunked file transfer with the following design:

- **Chunk size:** 16 MB (configurable, must be multiple of 4KB)
- **Protocol:** Sender announces total file size and number of chunks; receiver acknowledges each chunk; sender marks confirmed chunks
- **Checksums:** SHA-256 per chunk, sent alongside the chunk data; receiver verifies and rejects corrupted chunks
- **Resume:** On reconnection after interruption, receiver reports last confirmed chunk index; sender resumes from that chunk
- **Compression:** Optional zstd compression per chunk (enabled via config), negotiated during transfer setup
- **Small files:** Files under 1 MB bypass chunking and are sent inline as a single message (avoids chunking overhead)

## Consequences
- **Positive:** Reliable large file transfers with resume capability; per-chunk checksums detect corruption early; optional compression reduces bandwidth
- **Negative:** More complex protocol (chunk negotiation, acknowledgments, state tracking); slight overhead for small files (bypassed via inline path);
- **Risks:** Chunk boundary alignment issues with delta sync (see ADR-0007); memory usage proportional to chunk size

## Alternatives Considered
- **Single-stream:** Simple but fragile; no resume, no per-chunk verification
- **TCP-level only:** Relies on TCP checksums (weak — 16-bit) and retransmission; doesn't survive application-level restarts or connection re-establishment
