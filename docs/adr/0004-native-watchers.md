# ADR-0004: Platform-Specific Native Filesystem Watchers

**Status:** Proposed
**Date:** 2026-07-08
**Author:** Roadmap Agent

## Context
The C++ daemon currently polls the filesystem at a fixed 1-second interval to detect file changes. This misses rapid sequences of changes (multiple writes within the same polling window), wastes CPU cycles when the filesystem is idle, and introduces up to 1 second of latency for change propagation.

## Decision
Implement a platform-abstracted watcher interface in C++ with native implementations:

- **Interface:** Abstract `Watcher` class with `start()`, `stop()`, `onChange(callback)` methods
- **Linux:** `inotify` — watch directories recursively, handle IN_CREATE, IN_MODIFY, IN_DELETE, IN_MOVED_FROM/TO
- **macOS:** `FSEvents` — use `kFSEventStreamCreateFlagFileEvents` for file-level granularity, coalesce events with a 50ms latency
- **Windows:** `ReadDirectoryChangesW` — use overlapped I/O with I/O completion port
- **Fallback:** Configurable polling watcher (used when native watcher is unavailable or disabled)
- **Debouncing:** All watchers debounce events with a configurable 100ms window to batch rapid changes

## Consequences
- **Positive:** Near-instant change detection (sub-100ms); significantly reduced CPU usage when idle; reliable capture of rapid file change sequences
- **Negative:** Increased code complexity (3 platform implementations + fallback); platform-specific edge cases (inotify watch limits, FSEvents latency tuning, Windows handle limits)
- **Risks:** Each platform has unique quirks (inotify requires recursive directory setup, FSEvents may coalesce too aggressively, ReadDirectoryChangesW has buffer overflow edge cases)

## Alternatives Considered
- **Continue polling:** Simple and portable but wastes CPU and misses rapid changes; acceptable only as fallback
- **libuv:** Cross-platform abstraction (used by Node.js) but adds an external C library dependency; wraps the same platform APIs we would use directly
- **librsync / libfswatch:** Mature watcher libraries but add build-time dependencies and reduce control over debouncing behavior
