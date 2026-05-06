# Repository Structure by Task Assignment

This structure aligns the monorepo to `quick_reference_task_assignment.md`.

## Ownership map

| Area | Primary language | Repository path(s) |
|---|---|---|
| Peer discovery | Go | `src/backend/go/internal/discovery` |
| Network transport and routing | Go | `src/backend/go/internal/network`, `src/backend/go/internal/routing` |
| Message protocol runtime | Go | `src/backend/go/internal/protocol` |
| Conflict detection/resolution | Go | `src/backend/go/internal/versioning/conflict` |
| Vector clocks | Go | `src/backend/go/internal/versioning/vectorclock` |
| Version metadata/history control | Go | `src/backend/go/internal/versioning/history` |
| Multi-repo coordination | Go | `src/backend/go/internal/coordination/*` |
| SQLite/state and IPC server | Go | `src/backend/go/internal/storage/sqlite`, `src/backend/go/internal/ipc/server` |
| File watching | C++ | `src/backend/cpp/src/fs/watcher` |
| File hashing | C++ | `src/backend/cpp/src/hash` |
| Atomic file operations | C++ | `src/backend/cpp/src/fileops/atomic` |
| Permissions | C++ | `src/backend/cpp/src/fileops/permissions` |
| UI and interaction | Java | `src/frontend/java/src/main/java/com/actionlearning/p2p/ui` |
| Java integration bridge | Java | `src/frontend/java/src/main/java/com/actionlearning/p2p/bridge` |
| Cross-language contracts | Shared | `src/shared/protocol`, `src/shared/ipc` |

## Notes

- This is a working boundary, not a permanent freeze.
- `p2p_implementation_guide.md` remains reference-only until implementation decisions are finalized.
- Any responsibility shifts should update this file and `src/README.txt` together.

