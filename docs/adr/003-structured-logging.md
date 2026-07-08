# ADR-003: Structured Logging Strategy

**Date:** 2026-07-08
**Context:** Architecture Team

---

## Context

The system has three different runtimes (Go, C++, Java), each with its own
logging capabilities. Without a coordinated logging strategy, debugging and
operations become difficult — logs are in different formats, with different
timestamps, and no consistent context.

Key requirements:
- **Structured format** for machine parsing (JSON)
- **Level support** (debug, info, warn, error)
- **Component attribution** (which runtime produced the log)
- **Low overhead** — logging should not impact sync performance
- **Forward compatibility** — C++ daemon logs are captured by Go's stdout

## Decision

We will use **zerolog** for the Go coordinator, with **JSON format output** as the
primary format. C++ daemon logs are piped through Go and reformatted.

### Go: zerolog

```go
import "github.com/rs/zerolog/log"

// Usage
log.Info().
    Str("component", "network").
    Str("peer_id", peerID).
    Msg("connection established")
```

**Output:**
```json
{"level":"info","time":"2026-07-08T10:00:00Z","component":"network","peer_id":"alice","message":"connection established"}
```

### C++: Reformatted via Go

The C++ daemon writes plain-text to stdout/stderr. The Go coordinator captures
this output, prefixes it with RFC 3339 timestamps, and forwards it to the
structured log.

### Java: java.util.logging → reformatted

Java uses standard `java.util.logging` (JUL) for its output. The Go coordinator
captures this similarly to C++ output.

### Log Levels

| Level | Purpose |
|-------|---------|
| `debug` | Detailed debugging information (file changes, heartbeats) |
| `info` | Normal operational messages (peers connected, transfers completed) |
| `warn` | Unexpected but recoverable issues (TOFU mismatch, retry) |
| `error` | Errors that require operator intervention (TLS handshake failure, disk full) |

### Log Files

| Component | File | Format |
|-----------|------|--------|
| Go coordinator | `/tmp/p2p_go.log` | JSON (zerolog) |
| Java frontend | `/tmp/p2p_java.log` | Plain text (JUL) |
| C++ daemon | Captured by Go → same log | Reformatted to JSON |

## Consequences

**Positive:**
- All logs are in a consistent JSON format for ingestion by ELK, Loki, etc.
- Log levels allow filtering in production vs. development
- Component attribution makes debugging across runtimes easier
- No heavy dependency for Go (zerolog is zero-allocation on hot path)

**Negative:**
- JSON is less human-readable than plain text for terminal viewing
- C++ and Java logs need reformatting, adding complexity
- Double logging: C++/Java write their own files, Go writes its own

**Mitigation:**
- `LOG_FORMAT=text` can be set for human readability during development
- Log reformatting is a simple prefix/JSON wrap, not heavy processing

## Status

Accepted

## References
- Deployment Guide: Log Aggregation section
- Environment variables `LOG_LEVEL`, `LOG_FORMAT`