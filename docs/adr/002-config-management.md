# ADR-002: Configuration File Format and Precedence

**Date:** 2026-07-08
**Context:** Architecture Team

---

## Context

The system has numerous configuration parameters: peer identity, network ports,
TLS settings, logging preferences, heartbeat intervals, etc. These need to be
configurable across different deployment scenarios (desktop app, Docker, CI).

Different contributors and operators have different preferences: some prefer
environment variables (12-factor style), others prefer config files for
complex configurations.

## Decision

We will support multiple configuration sources with clear **precedence rules**:

### Precedence

```
CLI flags (highest priority)
    ↓
Environment variables
    ↓
Configuration file (~/.p2p/config.toml)
    ↓
Built-in defaults (lowest priority)
```

### Configuration File Format

We will use **TOML** for the configuration file.

```toml
[peer]
id = "my-laptop"
port = 9876

[tls]
enabled = true
cert_dir = "~/.p2p/certs"

[logging]
level = "info"
format = "json"
```

### Rationale

| Format | Pros | Cons |
|--------|------|------|
| TOML | Clear, unambiguous, comments, standard library in Go ecosystem | Less widely known than YAML |
| YAML | Widely known | Space-sensitive, complex features, security concerns (`!!python/`) |
| JSON | Ubiquitous | No comments, verbose, trailing comma issues |
| HCL | HashiCorp ecosystem | Unnecessary dependency, limited tooling |

TOML was chosen because of Go's strong ecosystem support (`BurntSushi/toml`),
its unambiguous syntax (no indentation problems), and support for comments.

### Environment Variable Mapping

All TOML keys have corresponding environment variables:

| TOML Path     | Environment Variable    |
|---------------|------------------------|
| `peer.id`     | `PEER_ID`              |
| `peer.port`   | `P2P_PORT`             |
| `tls.enabled` | `P2P_TLS_ENABLED`      |
| `tls.cert_dir`| `P2P_TLS_CERT_DIR`     |
| `logging.level` | `LOG_LEVEL`          |
| `logging.format` | `LOG_FORMAT`        |

### Implementation

```go
type Config struct {
    Peer   PeerConfig   `toml:"peer"`
    TLS    TLSConfig    `toml:"tls"`
    Logging LogConfig   `toml:"logging"`
}

// LoadConfig loads with precedence: defaults → file → env → flags
func LoadConfig(path string) (*Config, error)
```

## Consequences

**Positive:**
- Users can choose their preferred configuration method
- Docker deployments use environment variables (12-factor)
- Desktop users can use a config file
- Development environments rely on defaults

**Negative:**
- Slightly more complex startup logic
- Need to maintain both config file and env var documentation

## Status

Accepted. Implementation: `pkg/config/` package in Go coordinator.

## References

- Deployment Guide: Configuration Reference
- `P2P_TLS_ENABLED`, `P2P_PORT`, etc. environment variables