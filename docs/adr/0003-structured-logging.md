# ADR-0003: Structured Logging Strategy

**Status:** Proposed
**Date:** 2026-07-08
**Author:** Roadmap Agent

## Context
All logging currently uses `fmt.Println`, `log.Printf`, and similar unstructured output. There are no log levels, no JSON formatting, no correlation IDs, and no way to filter or route logs programmatically. This makes debugging in production difficult and prevents integration with log aggregation systems.

## Decision
Adopt zerolog for structured logging with the following conventions:

- **Output format:** JSON lines (newline-delimited JSON) for machine parseability
- **Log levels:** debug, info, warn, error, fatal (no panic level)
- **Component tags:** Each log line includes a `component` field (e.g., `coordinator`, `daemon-ipc`, `p2p`, `watcher`)
- **Correlation IDs:** A request-scoped correlation ID is propagated through the IPC and P2P layers, included in all related log lines
- **Time format:** RFC3339 with millisecond precision
- **Output destination:** stderr (by convention); log file path configurable via config file
- **Pretty-printing:** In development mode, a console writer with colorized output

## Consequences
- **Positive:** Machine-parseable logs enable integration with Logstash, Grafana Loki, or any JSON log aggregator; log levels reduce noise in production; correlation IDs enable tracing requests across components
- **Negative:** Approximately 2-3x slower than simple printf (due to JSON serialization); larger log volume on disk
- **Risks:** Performance impact in hot paths — ensure log calls use zerolog's event pooling and avoid allocation-heavy patterns

## Alternatives Considered
- **zap (uber-go):** Faster than zerolog in some benchmarks, but has a more complex API with two modes (sugared vs. unsugared); higher learning curve
- **logrus:** Deprecated by the author; no longer receives feature updates
- **slog (Go 1.21 stdlib):** Newer and less mature ecosystem; limited third-party integration; no built-in event pooling
