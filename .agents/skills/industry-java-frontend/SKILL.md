---
name: industry-java-frontend
description: Polished JavaFX desktop UI with conflict resolution, notifications, and proper error handling.
---

# Java Frontend Agent

**Role:** Transform the JavaFX frontend into a professional desktop application
**Branch:** `ws/java-frontend` (branched from `master`)
**Merge target:** `industry-shipping`
**Phase:** 1

## Work Items (in priority order)

1. **Conflict Resolution UI**
   - Create a dedicated conflict resolution dialog/view
   - When `conflict_detected` IPC message arrives, show:
     - File name, path, conflicting peers
     - Local version vs remote version (size, hash, timestamp)
     - Action buttons: "Keep Local", "Accept Remote", "Merge"
   - Send `resolution_applied` IPC message back to Go
   - Track unresolved conflicts in a sidebar/badge

2. **Connection Status & Sync Progress**
   - Add connection status indicator (green/orange/red dot for each peer)
   - Show sync progress bar for active file transfers
   - Display queue depth: "3 files waiting to sync"
   - Add last-synced timestamp per file/repo
   - Handle peer disconnection gracefully (show "Peer offline" state)

3. **System Tray Integration**
   - Minimize to system tray on close (configurable)
   - Show sync status in tray icon (syncing/idle/conflict/error)
   - Tray menu: Show window, Pause sync, Quit
   - Desktop notifications on: sync complete, conflict detected, error

4. **Error Handling & Reconnection**
   - Detect Go coordinator crash and show reconnect UI
   - Auto-reconnect with progress indicator
   - Display detailed error messages (not just "Connection lost")
   - Graceful degradation: show cached data when offline
   - Handle all IPC error scenarios (timeout, malformed response, Go not responding)

5. **Loading States & UX Polish**
   - Loading spinners for all async operations (repo list, status refresh)
   - Empty states: "No repositories tracked yet. Click + to add one."
   - Disable buttons during operations to prevent double-clicks
   - Keyboard shortcuts: Cmd+N (add repo), Cmd+R (refresh), Cmd+W (close)
   - Smooth animations for list updates

6. **Application Settings Dialog**
   - Settings window: General, Network, Appearance
   - Configurable: IPC socket path, P2P port, watch directories
   - Theme: system default / light / dark (persist choice)
   - Auto-launch on startup (checkbox)
   - Log level selector (for troubleshooting)

7. **Startup Wizard / First-Run Experience**
   - First launch: welcome screen explaining P2P sync
   - Step 1: Name your device (peer ID)
   - Step 2: Add a repository to sync
   - Step 3: Discover or enter peer info
   - Option to skip and use defaults

8. **Repository Management**
   - Improved add repo dialog with path picker (directory chooser)
   - Remove repo with confirmation dialog
   - Edit repo settings (sync mode, exclude patterns)
   - Show repo health: last sync, peer count, file count

9. **Java JUnit 5 Test Suite**
   - `IpcBridgeTest`: connect, disconnect, message send/receive, reconnect
   - `RepositoryListControllerTest`: add/remove/select repo
   - `RepoStatusControllerTest`: status display, conflict handling
   - `HelloControllerTest`: navigation, theme toggle
   - FXML binding verification (all fx:id and onAction match controllers)

10. **Windows & macOS Polish**
    - macOS: notarized DMG, menu bar integration, native file dialogs
    - Windows: MSI installer, Start Menu integration, file association

## Relevant Files
- `src/frontend/main/java/org/codehaus/mojo/frontendtest/` — All Java sources
- `src/frontend/main/resources/org/codehaus/mojo/frontendtest/` — FXML and CSS
- `pom.xml` — Add dependencies (ControlsFX, Ikonli already declared)
- `src/backend/go/pkg/protocol/messages.go` — Conflict resolution messages

## Verification
- `./mvnw compile` — must compile
- `./mvnw test` — JUnit tests pass
- Launch app and verify: conflict dialog appears on conflict
- Toggle theme: dark/light persists across restart
- Kill Go coordinator: app shows reconnect UI and recovers
