# ADR-0008: Conflict Resolution User Interface

**Status:** Proposed
**Date:** 2026-07-08
**Author:** Roadmap Agent

## Context
Conflicts are detected by the Go coordinator (divergent file versions from different peers), but the user currently has no interface to resolve them. The system either silently keeps one version (data loss risk) or leaves the file in a conflicted state that the user must manually untangle.

## Decision
Implement a dedicated conflict resolution dialog in the JavaFX frontend:

- **Dialog features:**
  - List all conflicted files with paths, timestamps, and peer sources
  - For each conflict, three resolution options:
    - **Keep Local:** Discard remote version, mark local as canonical
    - **Accept Remote:** Overwrite local with remote version
    - **Merge:** Open a three-pane diff view (local, remote, merged)
  - File diff view shows line-by-line differences (for text files) or metadata comparison (for binary files)
  - User can select "Apply to all" for batch resolution of similar conflicts
- **IPC messages:** New message types in the Go ↔ Java protocol:
  - `ConflictDetected` (Go → Java): Notify frontend of new conflict
  - `ConflictResolution` (Java → Go): User's choice (keep/accept/merge)
  - `ConflictResolved` (Go → Java): Confirmation that resolution was applied
  - `ConflictList` (Go → Java): Full list of outstanding conflicts
- **State management:** Conflicts are persisted in SQLite so they survive coordinator restarts

## Consequences
- **Positive:** Users can actively manage conflicts rather than losing data; visible conflict state prevents silent data loss; batch resolution handles bulk scenarios
- **Negative:** Significant UI complexity (diff viewer is non-trivial); new IPC message types add protocol surface area; merge option requires basic text merge logic
- **Risks:** Binary files cannot be merged line-by-line — must fall back to keep/accept only; merge conflicts within the same file opened by multiple users simultaneously

## Alternatives Considered
- **Auto-resolve (last-writer-wins):** Simple but dangerous — silently discards concurrent changes; violates the "no data loss" principle
- **Ignore conflicts:** Leaves conflicted files on disk with no user visibility; users may not notice until data is lost
- **External merge tool integration:** More powerful but creates a dependency on external tools (Beyond Compare, Kaleidoscope, etc.) and complicates the cross-platform story
