# ADR-0007: Delta Sync Strategy for Large File Updates

**Status:** Proposed
**Date:** 2026-07-08
**Author:** Roadmap Agent

## Context
When a large file (e.g., a database dump or binary asset) is modified, the current implementation transfers the entire file even if only a few bytes changed. This wastes bandwidth and time, particularly for files in the 100MB+ range where a small edit might change only a single block.

## Decision
Implement rsync-style delta synchronization:

- **Algorithm:** Rolling hash (Adler-32) for block boundary detection + SHA-256 per block for content verification
- **Block size:** 4 KB for files < 64 MB, 16 KB for larger files (heuristic based on file size)
- **Protocol:**
  1. Receiver computes block checksums for its version of the file and sends them to the sender
  2. Sender computes rolling checksums over its version, identifies matching blocks
  3. Sender transmits only non-matching blocks (changed content) and a block index map for unchanged blocks
  4. Receiver reconstructs the file from unchanged blocks (local) + changed blocks (from sender)
- **Fallback:** If delta sync overhead exceeds 50% of full file size, fall back to full file transfer
- **Integration:** Operates on top of chunked transfer (ADR-0006) — each block is sent as part of a chunk

## Consequences
- **Positive:** Dramatically reduced bandwidth for small changes to large files; efficient use of network resources; transparent to the user
- **Negative:** Significant implementation complexity (rolling hash, block matching algorithm, two-phase protocol); CPU cost for hash computation on both sides; memory overhead for block index map
- **Risks:** Rolling hash collisions (Adler-32 is weak — mitigated by SHA-256 verification of matched blocks); block alignment sensitivity; edge cases with very small files (fallback handles)

## Alternatives Considered
- **Full file always:** Simple and correct but wasteful for large files with small changes
- **zstd compression only:** Reduces transfer size but still requires sending the entire file; doesn't exploit file similarity between versions
- **rsync binary (external):** Would require bundling rsync and managing its lifecycle — adds OS dependency, inconsistent availability on Windows
