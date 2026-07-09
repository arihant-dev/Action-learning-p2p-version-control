# ADR-004: Native Filesystem Watchers Platform Abstraction

**Date:** 2026-07-08
**Context:** Architecture Team

---

## Context

The C++ daemon needs to detect file changes in tracked directories. Different
platforms provide different native APIs for this:

| Platform | Native API | Granularity | Overhead |
|----------|-----------|-----------|----------|
| Linux    | inotify   | Per-file  | Low      |
| macOS    | FSEvents | Per-volume | Very low |
| Windows  | ReadDirectoryChangesW | Per-directory | Low |

Currently, the C++ daemon uses a **polling-based approach** (checking every 1s).
This is inefficient: it uses CPU even when no changes occur, and has a 1-second
latency on change detection.

## Decision

We will implement a **platform abstraction layer** that uses native watchers
where available, with polling as fallback.

### Architecture

```
┌─────────────────────────────────────┐
│       FileSystemWatcher (abstract)   │
├─────────────────────────────────────┤
│  start() / stop() / on_change()     │
└────────────────┬────────────────────┘
                 │
      ┌──────────┼──────────┐
      ↓          ↓          ↓
┌──────────┐ ┌────────┐ ┌──────────────┐
│ Linux    │ │ macOS  │ │ Windows      │
│ inotify  │ │ FSEvents│ │ ReadDirChangesW│
└──────────┘ └────────┘ └──────────────┘
      └─ polling fallback ────┘
```

### Implementation Pattern

```cpp
class FileSystemWatcher {
    ChangeCallback callback_;
    
    bool start() {
#if defined(__linux__)
        return start_inotify();
#elif defined(__APPLE__)
        return start_fsevents();
#elif defined(_WIN32)
        return start_windows();
#else
        return start_polling(); // fallback
#endif
    }

    // Platform-specific implementations
    bool start_inotify();   // src/fs/watcher_linux.cpp
    bool start_fsevents();  // src/fs/watcher_macos.cpp
    bool start_windows();   // src/fs/watcher_windows.cpp
    bool start_polling();   // src/fs/watcher_poll.cpp
};
```

### Polling Fallback

For platforms without native support (or when native APIs fail), a polling
fallback checks the directory every `POLL_INTERVAL` milliseconds (default: 1000ms).
This is the current implementation and remains functional.

### Build System

CMake selects the correct platform source:

```cmake
if(UNIX AND NOT APPLE)
    set(PLATFORM_SOURCES src/fs/watcher_linux.cpp)
elseif(APPLE)
    set(PLATFORM_SOURCES src/fs/watcher_macos.cpp)
elseif(WIN32)
    set(PLATFORM_SOURCES src/fs/watcher_windows.cpp)
endif()
```

## Consequences

**Positive:**
- Near-instant file change detection (milliseconds vs. 1s polling)
- Lower CPU usage (no busy-wait polling)
- Platform-optimized implementations

**Negative:**
- More code to maintain (3+ platform implementations)
- Testing is more complex (need platform-specific test environments)
- FSEvents requires a run loop on macOS, adding threading complexity
- inotify has directory descriptor limits on Linux (can be tuned via `/proc/sys/fs/inotify/max_user_watches`)

## Status

Accepted

## References
- Implementation Guide: File System Watcher Interface
- `src/backend/cpp/src/filesystem_watcher.cpp` (current polling implementation)