package sync

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"p2p/pkg/ipc"
	"p2p/pkg/network"
	"p2p/pkg/protocol"
	"p2p/pkg/storage/sqlite"
	"p2p/pkg/transfer"
	"p2p/pkg/versioning"
)

// SyncCoordinator coordinates multi-repository synchronization across the local
// C++ daemon and P2P network.
type pendingDownload struct {
	size   int64
	repoID string
	mode   uint32
}

type SyncCoordinator struct {
	db               *sqlite.DB
	ipcServer        *ipc.IpcServer
	connMgr          *network.ConnectionManager
	detector         *versioning.ConflictDetector
	queue            *SyncQueue
	transferMgr      *transfer.FileTransferManager
	localPeerID      string
	mu               sync.RWMutex
	workers          map[string]chan struct{} // repoID -> stop channel
	repoSemaphores   map[string]chan struct{} // per-repository concurrency control
	globalSem        chan struct{}            // global parent semaphore
	stopChan         chan struct{}
	wg               sync.WaitGroup
	pendingDownloads map[string]pendingDownload // path:hash -> pendingDetails
	lamportClock     *versioning.LamportClock
	vectorClock      *versioning.VectorClock
}

// NewSyncCoordinator creates a new SyncCoordinator.
func NewSyncCoordinator(
	db *sqlite.DB,
	ipcServer *ipc.IpcServer,
	connMgr *network.ConnectionManager,
	localPeerID string,
) *SyncCoordinator {
	globalConcurrency := 16
	if s := os.Getenv("P2P_GLOBAL_CONCURRENCY"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			globalConcurrency = v
		}
	}
	return &SyncCoordinator{
		db:               db,
		ipcServer:        ipcServer,
		connMgr:          connMgr,
		detector:         versioning.NewConflictDetector(),
		queue:            NewSyncQueue(),
		transferMgr:      transfer.NewFileTransferManager(ipcServer),
		localPeerID:      localPeerID,
		workers:          make(map[string]chan struct{}),
		repoSemaphores:   make(map[string]chan struct{}),
		globalSem:        make(chan struct{}, globalConcurrency),
		stopChan:         make(chan struct{}),
		pendingDownloads: make(map[string]pendingDownload),
		lamportClock:     versioning.NewLamportClock(),
		vectorClock:      versioning.NewVectorClock(),
	}
}

// Start boots the coordinator, starts database repos, and begins processing queues.
func (sc *SyncCoordinator) Start() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Load persisted vector clocks from database
	if err := sc.loadVectorClocks(); err != nil {
		log.Printf("[SyncCoordinator] Warning: failed to load vector clocks: %v\n", err)
	}

	// Restore Lamport clock so newly assigned versions never regress
	if maxVer, err := sc.db.Metadata().MaxVersion(); err == nil && maxVer > 0 {
		sc.lamportClock = versioning.NewLamportClockAt(uint64(maxVer))
		log.Printf("[SyncCoordinator] Restored Lamport clock to %d\n", maxVer)
	}

	// Load existing active repositories and start workers
	repos, err := sc.db.Repositories().List()
	if err != nil {
		return fmt.Errorf("list database repos: %w", err)
	}

	for _, repo := range repos {
		if repo.Status == "active" {
			sc.startRepoWorkerLocked(repo.ID)
		}
	}
	sc.wg.Add(1)
	go sc.queueProcessorLoop()

	return nil
}

// Stop gracefully shuts down all repository workers and transfer streams.
func (sc *SyncCoordinator) Stop() {
	sc.mu.Lock()
	close(sc.stopChan)
	for repoID, stopCh := range sc.workers {
		close(stopCh)
		delete(sc.workers, repoID)
	}
	sc.mu.Unlock()

	sc.wg.Wait()
}

func (sc *SyncCoordinator) startRepoWorkerLocked(repoID string) {
	if _, exists := sc.workers[repoID]; exists {
		return
	}
	stopCh := make(chan struct{})
	sc.workers[repoID] = stopCh

	repoConcurrency := 2
	if s := os.Getenv("P2P_REPO_CONCURRENCY"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			repoConcurrency = v
		}
	}
	if _, exists := sc.repoSemaphores[repoID]; !exists {
		sc.repoSemaphores[repoID] = make(chan struct{}, repoConcurrency)
	}

	sc.wg.Add(1)
	go func() {
		defer sc.wg.Done()
		log.Printf("[SyncCoordinator] Started sync worker for repo %s\n", repoID)

		// 1. Fetch repository details to get watch path
		repo, err := sc.db.Repositories().Get(repoID)
		if err != nil || repo == nil {
			log.Printf("[SyncCoordinator] Error: Repository %s not found in DB. Cannot start C++ daemon.\n", repoID)
			<-stopCh
			log.Printf("[SyncCoordinator] Stopped sync worker for repo %s\n", repoID)
			return
		}

		// 2. Perform initial directory scan and index existing local files
		if err := sc.ScanAndIndexLocalFiles(repoID, repo.LocalPath); err != nil {
			log.Printf("[SyncCoordinator] Warning: Initial directory scan failed for repo %s: %v\n", repoID, err)
		}

		// 3. Find C++ daemon binary
		cppExe := "./" + daemonBinaryName("p2p_daemon")
		if execPath, errExec := os.Executable(); errExec == nil {
			peerCpp := filepath.Join(filepath.Dir(execPath), daemonBinaryName("p2p_daemon"))
			if _, errStat := os.Stat(peerCpp); errStat == nil {
				cppExe = peerCpp
			}
		}

		if _, err := os.Stat(cppExe); os.IsNotExist(err) {
			candidates := []string{
				"src/backend/cpp/build/bin/" + daemonBinaryName("p2p_daemon"),
				"src/backend/cpp/build/bin/Release/" + daemonBinaryName("p2p_daemon"),
				"src/backend/cpp/build/Release/" + daemonBinaryName("p2p_daemon"),
				"src/backend/cpp/build/bin/Debug/" + daemonBinaryName("p2p_daemon"),
				"src/backend/cpp/build/Debug/" + daemonBinaryName("p2p_daemon"),
				"../cpp/build/bin/" + daemonBinaryName("p2p_daemon"),
				"build/bin/" + daemonBinaryName("p2p_daemon"),
				"build/" + daemonBinaryName("p2p_daemon"),
			}
			for _, c := range candidates {
				if _, errStat := os.Stat(c); errStat == nil {
					cppExe = c
					break
				}
			}
		}

		// 3. Compile C++ daemon if missing and CMake project exists
		if _, err := os.Stat(cppExe); os.IsNotExist(err) {
			if _, errCmake := os.Stat("src/backend/cpp/CMakeLists.txt"); errCmake == nil {
				log.Println("[SyncCoordinator] C++ daemon binary missing. Attempting build...")
				_ = exec.Command("cmake", "-B", "src/backend/cpp/build", "-S", "src/backend/cpp").Run()
				_ = exec.Command("cmake", "--build", "src/backend/cpp/build").Run()
				builtCandidates := []string{
					"src/backend/cpp/build/bin/" + daemonBinaryName("p2p_daemon"),
					"src/backend/cpp/build/bin/Release/" + daemonBinaryName("p2p_daemon"),
					"src/backend/cpp/build/Release/" + daemonBinaryName("p2p_daemon"),
				}
				for _, bc := range builtCandidates {
					if _, errStat := os.Stat(bc); errStat == nil {
						cppExe = bc
						break
					}
				}
			}
		}

		// 4. Start C++ daemon process in a new process group
		var cmd *exec.Cmd
		if _, err := os.Stat(cppExe); err == nil {
			log.Printf("[SyncCoordinator] Spawning C++ daemon: %s %s %s %s\n", cppExe, repoID, repo.LocalPath, sc.ipcServer.SocketPath())
			cmd = exec.Command(cppExe, repoID, repo.LocalPath, sc.ipcServer.SocketPath())
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			// Use process group so we can kill the entire group on shutdown (Unix only)
			setProcessGroup(cmd)
			if err := cmd.Start(); err != nil {
				log.Printf("[SyncCoordinator] Error: Failed to start C++ daemon: %v\n", err)
			}
		} else {
			log.Println("[SyncCoordinator] Warning: C++ daemon binary not found. Watcher daemon not started.")
		}

		<-stopCh

		// 5. Clean up C++ daemon process (kill entire process group)
		if cmd != nil && cmd.Process != nil {
			log.Printf("[SyncCoordinator] Terminating C++ daemon for repo %s...\n", repoID)
			// Try graceful termination first
			_ = killProcessGroup(cmd, true)
			// Wait with timeout
			done := make(chan struct{})
			go func() {
				_ = cmd.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				log.Printf("[SyncCoordinator] Force killing C++ daemon for repo %s...\n", repoID)
				_ = killProcessGroup(cmd, false)
				_ = cmd.Wait()
			}
		}

		log.Printf("[SyncCoordinator] Stopped sync worker for repo %s\n", repoID)
	}()
}

// AddRepository adds a repository to track and sync.
func (sc *SyncCoordinator) AddRepository(repoID, localPath string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	cleanedPath := localPath
	if cp, err := filepath.EvalSymlinks(localPath); err == nil {
		if abs, err := filepath.Abs(cp); err == nil {
			cleanedPath = abs
		}
	} else {
		if abs, err := filepath.Abs(localPath); err == nil {
			cleanedPath = abs
		}
	}

	repo := sqlite.Repository{
		ID:        repoID,
		LocalPath: cleanedPath,
		Status:    "active",
		SyncMode:  "auto",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}

	if err := sc.db.Repositories().Save(&repo); err != nil {
		return err
	}

	sc.startRepoWorkerLocked(repoID)
	return nil
}

// RemoveRepository untracks a repository and cleans up its tasks.
func (sc *SyncCoordinator) RemoveRepository(repoID string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if err := sc.db.Repositories().Delete(repoID); err != nil {
		return err
	}

	if stopCh, exists := sc.workers[repoID]; exists {
		close(stopCh)
		delete(sc.workers, repoID)
	}

	sc.queue.RemoveRepository(repoID)
	return nil
}

// HandleLocalFileChanged handles an IPC file changed event from C++.
func (sc *SyncCoordinator) HandleLocalFileChanged(repoID string, payload *protocol.FileChangedPayload) error {
	sc.mu.RLock()
	_, active := sc.workers[repoID]
	sc.mu.RUnlock()

	if !active {
		return fmt.Errorf("repository %s is not active", repoID)
	}

	// 0. Normalize absolute path to relative path, then to forward slashes for cross-platform compat
	if filepath.IsAbs(payload.Path) {
		repo, err := sc.db.Repositories().Get(repoID)
		if err == nil && repo != nil {
			if rel, err := filepath.Rel(repo.LocalPath, payload.Path); err == nil {
				payload.Path = rel
			}
		}
	}
	payload.Path = filepath.ToSlash(payload.Path)

	// Skip temporary files (.tmp) created during download — they are
	// internal to the transfer and should never enter the metadata DB.
	if strings.HasSuffix(payload.Path, ".tmp") {
		log.Printf("[SyncCoordinator] Ignoring temp file change: %s\n", payload.Path)
		return nil
	}

	// 1. Fetch current database metadata
	existing, err := sc.db.Metadata().Get(repoID, payload.Path)
	if err == nil && existing != nil {
		// Ignore redundant file change notifications that match the recorded db state (e.g. from sync downloads).
		// Compare size and mode too — the hash alone is insufficient because a race in the C++ daemon
		// can produce size=0 with the correct hash (TOCTOU between file_size and sha256).
		if existing.Hash == payload.Hash && existing.Size == payload.Size && existing.Mode == uint32(payload.Mode) && existing.IsDeleted == (payload.Action == "delete") {
			log.Printf("[SyncCoordinator] Ignoring redundant file change for: %s (hash matches db)\n", payload.Path)
			return nil
		}
	}

	// 2. Determine per-file version and tick vector clock for causal ordering
	var nextVersion int64 = 1
	if existing != nil {
		nextVersion = existing.Version + 1
	}
	sc.lamportClock.Tick()
	sc.vectorClock.Tick(sc.localPeerID)

	// 3. Save new metadata state to SQLite
	meta := sqlite.FileMetadata{
		RepositoryID:      repoID,
		Filepath:          payload.Path,
		Hash:              payload.Hash,
		Size:              payload.Size,
		Version:           nextVersion,
		LocalLastModified: payload.ModifiedTime,
		IsDeleted:         payload.Action == "delete",
		UpdatedAt:         time.Now().Unix(),
		Mode:              payload.Mode,
	}

	if err := sc.db.Metadata().Save(&meta); err != nil {
		return fmt.Errorf("failed to save file metadata: %w", err)
	}

	// 4. Log to sync history
	histEvent := sqlite.SyncEvent{
		EventID:      fmt.Sprintf("evt_%d_%s", time.Now().UnixNano(), repoID),
		RepositoryID: repoID,
		FilePath:     payload.Path,
		PeerID:       sc.localPeerID,
		Timestamp:    time.Now().Unix(),
		EventType:    "local_change",
		Status:       "success",
	}
	_ = sc.db.History().LogEvent(&histEvent)

	// 5. Broadcast change to all connected network peers
	// Skip broadcast for size=0 files (C++ may fire Created event before file is fully written).
	// A subsequent Modified event with the real size will trigger the actual broadcast.
	if payload.Size > 0 || payload.Action == "delete" {
		p2pMsg := &ipc.Message{
			Version:   "1.0",
			Type:      "file_metadata_update",
			Source:    "go",
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		}
		updatePayload := map[string]interface{}{
			"repo_id":       repoID,
			"path":          payload.Path,
			"hash":          payload.Hash,
			"size":          payload.Size,
			"version":       uint64(nextVersion),
			"modified_time": payload.ModifiedTime,
			"is_deleted":    payload.Action == "delete",
			"mode":          payload.Mode,
			"vector_clock":  sc.vectorClock.AsMap(),
		}
		payloadBytes, err := json.Marshal(updatePayload)
		if err != nil {
			return fmt.Errorf("marshal metadata update: %w", err)
		}
		p2pMsg.Payload = payloadBytes

		sc.connMgr.Broadcast(p2pMsg)
	}
	return nil
}

// HandlePeerMetadataUpdate processes version changes received from a remote peer.
func (sc *SyncCoordinator) HandlePeerMetadataUpdate(peerID string, repoID string, update map[string]interface{}) {
	path, _ := update["path"].(string)
	path = filepath.ToSlash(path)

	// Ignore metadata for temporary files — they are transfer internals.
	if strings.HasSuffix(path, ".tmp") {
		return
	}

	// Reject path traversal attempts from peers
	if filepath.IsAbs(path) || strings.Contains(path, "..") {
		log.Printf("[SyncCoordinator] Rejecting path traversal from peer %s: %s\n", peerID, path)
		return
	}
	hash, _ := update["hash"].(string)
	sizeVal, _ := update["size"].(float64)
	verVal, _ := update["version"].(float64)
	modTimeVal, _ := update["modified_time"].(float64)
	isDeleted, _ := update["is_deleted"].(bool)
	modeVal, _ := update["mode"].(float64)
	remoteVC, _ := update["vector_clock"].(map[string]interface{})

	size := int64(sizeVal)
	version := uint64(verVal)
	modifiedTime := int64(modTimeVal)
	mode := uint32(modeVal)

	// Merge remote vector clock to preserve causal ordering
	if remoteVC != nil {
		vcMap := make(map[string]uint64, len(remoteVC))
		for k, v := range remoteVC {
			if fv, ok := v.(float64); ok {
				vcMap[k] = uint64(fv)
			}
		}
		sc.vectorClock.Merge(vcMap)
	}

	// 1. Get current local state
	localMeta, err := sc.db.Metadata().Get(repoID, path)
	var localVer uint64 = 0
	var localHash string = ""
	var localTime int64 = 0
	if err == nil && localMeta != nil {
		localVer = uint64(localMeta.Version)
		localHash = localMeta.Hash
		localTime = localMeta.LocalLastModified
	}

	localFV := versioning.FileVersion{
		Hash:           localHash,
		LamportVersion: localVer,
		Timestamp:      localTime,
		PeerID:         sc.localPeerID,
	}
	remoteFV := versioning.FileVersion{
		Hash:           hash,
		LamportVersion: version,
		Timestamp:      modifiedTime,
		PeerID:         peerID,
	}

	resolution := sc.detector.Resolve(localFV, remoteFV)

	// 2. Apply resolution
	switch resolution.Action {
	case versioning.AcceptRemote:
		// Witness remote Lamport clock for causal ordering
		sc.lamportClock.Witness(version)

		if isDeleted {
			// Save deletion tombstone to SQLite DB
			meta := sqlite.FileMetadata{
				RepositoryID:      repoID,
				Filepath:          path,
				Hash:              hash,
				Size:              size,
				Version:           int64(version),
				LocalLastModified: modifiedTime,
				IsDeleted:         true,
				UpdatedAt:         time.Now().Unix(),
				Mode:              mode,
			}
			_ = sc.db.Metadata().Save(&meta)

			// Tell C++ daemon to delete the file locally
			delMsg := &ipc.Message{
				Version:   "1.0",
				Type:      "sync_from_peer",
				Source:    "go",
				Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			}
			payload := map[string]interface{}{
				"peer_id":   peerID,
				"path":      path,
				"repo_id":   repoID,
				"is_delete": true,
				"hash":      hash,
				"timestamp": modifiedTime,
			}
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				log.Printf("[SyncCoordinator] Failed to marshal delete payload: %v\n", err)
			} else {
				delMsg.Payload = payloadBytes
				sc.ipcServer.SendMessage(delMsg)
			}
		} else {
			// Save remote metadata to SQLite DB first to prevent feedback loops on download completion
			meta := sqlite.FileMetadata{
				RepositoryID:      repoID,
				Filepath:          path,
				Hash:              hash,
				Size:              size,
				Version:           int64(version),
				LocalLastModified: modifiedTime,
				IsDeleted:         false,
				UpdatedAt:         time.Now().Unix(),
				Mode:              mode,
			}
			_ = sc.db.Metadata().Save(&meta)

			// Check if we already have a pending download for this file+hash
			sc.mu.RLock()
			_, hasPending := sc.pendingDownloads[path+":"+hash]
			sc.mu.RUnlock()
			if hasPending || sc.queue.HasPending(repoID, path, hash) {
				log.Printf("[SyncCoordinator] Download already pending for %s (hash %s), skipping duplicate\n", path, hash)
			} else {
				// Push download task
				task := &SyncTask{
					RepoID:    repoID,
					FilePath:  path,
					Type:      Download,
					Hash:      hash,
					Size:      size,
					Timestamp: time.Unix(modifiedTime, 0),
					PeerID:    peerID,
					Mode:      mode,
				}
				sc.queue.Push(task)
			}
		}

		if resolution.IsConflict {
			sc.logAndNotifyConflict(repoID, path, localFV, remoteFV)
		}

	case versioning.KeepLocal:
		// Remote is behind. Send our updated metadata to them.
		if resolution.IsConflict {
			sc.logAndNotifyConflict(repoID, path, localFV, remoteFV)
		} else {
			sc.sendMetadataUpdateToPeer(peerID, repoID, path, localMeta.Size, localVer, localHash, localTime, localMeta.IsDeleted, localMeta.Mode)
		}
	case versioning.Skip:
		// Identical hashes — normally nothing to do.
		// But if the file is missing on disk (e.g., after a failed download), re-download.
		if localMeta != nil && !localMeta.IsDeleted {
			repo, _ := sc.db.Repositories().Get(repoID)
			if repo != nil {
				absPath := filepath.Join(repo.LocalPath, path)
				if _, err := os.Stat(absPath); os.IsNotExist(err) {
					log.Printf("[SyncCoordinator] File %s missing on disk despite matching DB hash — re-downloading\n", path)
					sc.queue.Push(&SyncTask{
						RepoID:    repoID,
						FilePath:  path,
						Type:      Download,
						Hash:      hash,
						Size:      size,
						Timestamp: time.Unix(modifiedTime, 0),
						PeerID:    peerID,
						Mode:      mode,
					})
				}
			}
		}
	}
}

func (sc *SyncCoordinator) logAndNotifyConflict(repoID, path string, local, remote versioning.FileVersion) {
	// Log conflict event to history DB
	histEvent := sqlite.SyncEvent{
		EventID:      fmt.Sprintf("conflict_%d", time.Now().UnixNano()),
		RepositoryID: repoID,
		FilePath:     path,
		PeerID:       remote.PeerID,
		Timestamp:    time.Now().Unix(),
		EventType:    "conflict_detected",
		Status:       "pending",
	}
	_ = sc.db.History().LogEvent(&histEvent)

	// Inform C++ local daemon of conflict so user can resolve
	msg := &ipc.Message{
		Version:   "1.0",
		Type:      "conflict_detected",
		Source:    "go",
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}
	payload := protocol.ConflictDetectedPayload{
		Path: path,
		Versions: []protocol.VersionMetadata{
			{
				Hash:      local.Hash,
				Timestamp: local.Timestamp,
				VectorClock: map[string]uint64{
					local.PeerID: local.LamportVersion,
				},
				SourcePeer: local.PeerID,
			},
			{
				Hash:      remote.Hash,
				Timestamp: remote.Timestamp,
				VectorClock: map[string]uint64{
					remote.PeerID: remote.LamportVersion,
				},
				SourcePeer: remote.PeerID,
			},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[SyncCoordinator] Failed to marshal conflict payload: %v\n", err)
		return
	}
	msg.Payload = payloadBytes

	sc.ipcServer.SendMessage(msg)
}

func (sc *SyncCoordinator) sendMetadataUpdateToPeer(peerID, repoID, path string, size int64, version uint64, hash string, modifiedTime int64, isDeleted bool, mode uint32) {
	p2pMsg := &ipc.Message{
		Version:   "1.0",
		Type:      "file_metadata_update",
		Source:    "go",
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}
	updatePayload := map[string]interface{}{
		"repo_id":       repoID,
		"path":          path,
		"hash":          hash,
		"size":          size,
		"version":       version,
		"modified_time": modifiedTime,
		"is_deleted":    isDeleted,
		"mode":          mode,
		"vector_clock":  sc.vectorClock.AsMap(),
	}
	payloadBytes, err := json.Marshal(updatePayload)
	if err != nil {
		log.Printf("[SyncCoordinator] Failed to marshal metadata update to peer %s: %v\n", peerID, err)
		return
	}
	p2pMsg.Payload = payloadBytes

	_ = sc.connMgr.SendToPeer(peerID, p2pMsg)
}

// queueProcessorLoop continuously pops tasks from the SyncQueue and processes them.
func (sc *SyncCoordinator) queueProcessorLoop() {
	defer sc.wg.Done()

	for {
		select {
		case <-sc.stopChan:
			return
		default:
			task := sc.queue.Pop()
			if task == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			sc.mu.RLock()
			repoSem, hasRepoSem := sc.repoSemaphores[task.RepoID]
			sc.mu.RUnlock()
		if !hasRepoSem {
			log.Printf("[SyncCoordinator] No semaphore for repo %s, requeueing task\n", task.RepoID)
			sc.queue.Requeue(task)
			time.Sleep(100 * time.Millisecond)
			continue
		}

			select {
			case <-sc.stopChan:
				return
			case sc.globalSem <- struct{}{}:
			}

			select {
			case <-sc.stopChan:
				<-sc.globalSem
				return
			case repoSem <- struct{}{}:
			}

			go func(t *SyncTask) {
				defer func() {
					<-repoSem
					<-sc.globalSem
				}()
				sc.executeSyncTask(t)
			}(task)
		}
	}
}

func (sc *SyncCoordinator) executeSyncTask(task *SyncTask) {
	log.Printf("[SyncCoordinator] Executing sync task: %s %s (%d bytes)\n", task.Type, task.FilePath, task.Size)

	if task.Type == Download {
		if !sc.connMgr.IsConnected(task.PeerID) {
			log.Printf("[SyncCoordinator] Transfer failed: peer %s disconnected, requeueing\n", task.PeerID)
			sc.queue.Requeue(task)
			return
		}

		// Request file from peer
		reqMsg := &ipc.Message{
			Version:   "1.0",
			Type:      "file_request",
			Source:    "go",
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		}
		reqPayload := protocol.FileRequestPayload{
			Path: task.FilePath,
			Hash: task.Hash,
		}
		payloadBytes, err := json.Marshal(reqPayload)
		if err != nil {
			log.Printf("[SyncCoordinator] Failed to marshal file request: %v\n", err)
			return
		}
		reqMsg.Payload = payloadBytes

		// Register pending download details
		sc.mu.Lock()
		sc.pendingDownloads[task.FilePath+":"+task.Hash] = pendingDownload{
			size:   task.Size,
			repoID: task.RepoID,
			mode:   task.Mode,
		}
		sc.mu.Unlock()

		// Actually send file_request to remote peer!
		if err := sc.connMgr.SendToPeer(task.PeerID, reqMsg); err != nil {
			sc.mu.Lock()
			delete(sc.pendingDownloads, task.FilePath+":"+task.Hash)
			sc.mu.Unlock()
			log.Printf("[SyncCoordinator] Failed to send file_request to peer %s: %v, requeueing\n", task.PeerID, err)
			sc.queue.Requeue(task)
			return
		}
		
		log.Printf("[SyncCoordinator] Download request sent to peer %s for: %s\n", task.PeerID, task.FilePath)
	}
}

// HandleP2PMessage handles messages arriving from remote Go peers over the network.
func (sc *SyncCoordinator) HandleP2PMessage(peerID string, msg *ipc.Message) error {
	switch msg.Type {
	case "file_metadata_update":
		var payload map[string]interface{}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal metadata update: %w", err)
		}
		repoID, _ := payload["repo_id"].(string)
		sc.HandlePeerMetadataUpdate(peerID, repoID, payload)
		return nil

	case "file_request":
		var payload protocol.FileRequestPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal file request: %w", err)
		}
		if err := payload.Validate(); err != nil {
			return fmt.Errorf("invalid file_request from peer %s: %w", peerID, err)
		}

		// Look up the file in SQLite across repos to get repositoryID and size
		meta, err := sc.db.Metadata().GetByPathAndHash(payload.Path, payload.Hash)
		if err != nil || meta == nil {
			log.Printf("[SyncCoordinator] File request rejected: %s not found locally\n", payload.Path)
			resp := &ipc.Message{
				Version:   "1.0",
				Type:      "file_response",
				Source:    "go",
				Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			}
			respPayload, _ := json.Marshal(protocol.FileResponsePayload{
				Path:  payload.Path,
				Hash:  payload.Hash,
				Error: "file not found locally",
			})
			resp.Payload = respPayload
			_ = sc.connMgr.SendToPeer(peerID, resp)
			return nil
		}

		// File is found locally. Start upload session.
		transferID := fmt.Sprintf("up_%d_%s", time.Now().UnixNano(), meta.RepositoryID)
		// Resolve repo root to pass as basePath so inline reads come from the correct location
		basePath := ""
		if repoForUpload, _ := sc.db.Repositories().Get(meta.RepositoryID); repoForUpload != nil {
			basePath = repoForUpload.LocalPath
		}
		transferPort, inlineData, err := sc.transferMgr.StartUpload(transferID, meta.Filepath, basePath, meta.RepositoryID, peerID, meta.Hash, meta.Size)
		if err != nil {
			log.Printf("[SyncCoordinator] StartUpload failed for %s: %v\n", payload.Path, err)
			resp := &ipc.Message{
				Version:   "1.0",
				Type:      "file_response",
				Source:    "go",
				Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			}
			respPayload, _ := json.Marshal(protocol.FileResponsePayload{
				Path:  payload.Path,
				Hash:  payload.Hash,
				Error: err.Error(),
			})
			resp.Payload = respPayload
			_ = sc.connMgr.SendToPeer(peerID, resp)
			return nil
		}

		// Send file_response to remote peer with dynamic transfer port
		resp := &ipc.Message{
			Version:   "1.0",
			Type:      "file_response",
			Source:    "go",
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		}
		fileResp := protocol.FileResponsePayload{
			Path: payload.Path,
			Hash: payload.Hash,
		}
		if inlineData != "" {
			fileResp.ContentBase64 = inlineData
		} else {
			fileResp.TransferPort = transferPort
		}
		respPayload, _ := json.Marshal(fileResp)
		resp.Payload = respPayload
		_ = sc.connMgr.SendToPeer(peerID, resp)
		return nil

	case "file_response":
		var payload protocol.FileResponsePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal file response: %w", err)
		}
		if err := payload.Validate(); err != nil {
			return fmt.Errorf("invalid file_response from peer %s: %w", peerID, err)
		}

		if payload.Error != "" {
			log.Printf("[SyncCoordinator] Remote file request failed: %s\n", payload.Error)
			return nil
		}

		// Find the peer's host IP from their active connection
		peerConn := sc.connMgr.GetConnection(peerID)
		if peerConn == nil {
			log.Printf("[SyncCoordinator] Peer %s disconnected before transfer\n", peerID)
			return nil
		}
		remoteAddr := peerConn.RemoteAddr().String()
		host, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			host = remoteAddr // Fallback
		}

		// Look up expected size and repo ID from pending downloads map
		sc.mu.Lock()
		pDetails, exists := sc.pendingDownloads[payload.Path+":"+payload.Hash]
		if exists {
			delete(sc.pendingDownloads, payload.Path+":"+payload.Hash)
		}
		sc.mu.Unlock()

		if !exists {
			log.Printf("[SyncCoordinator] Pending download details not found for response: %s\n", payload.Path)
			return nil
		}

		if payload.ContentBase64 != "" {
			data, err := base64.StdEncoding.DecodeString(payload.ContentBase64)
			if err != nil {
				log.Printf("[SyncCoordinator] Failed to decode base64 content: %v\n", err)
				return nil
			}
			repo, err := sc.db.Repositories().Get(pDetails.repoID)
			if err != nil || repo == nil {
				log.Printf("[SyncCoordinator] Repository not found for inline write: %s\n", pDetails.repoID)
				return nil
			}

			// Reject path traversal on inline write
			if filepath.IsAbs(payload.Path) || strings.Contains(payload.Path, "..") {
				log.Printf("[SyncCoordinator] Rejecting path traversal on inline write: %s\n", payload.Path)
				return nil
			}

			// Verify SHA-256 against the expected hash before writing
			computedHash := fmt.Sprintf("%x", sha256.Sum256(data))
			if payload.Hash != "" && computedHash != payload.Hash {
				log.Printf("[SyncCoordinator] Inline file hash mismatch for %s: got %s, expected %s\n", payload.Path, computedHash, payload.Hash)
				return nil
			}

			absPath := filepath.Join(repo.LocalPath, payload.Path)
			err = os.WriteFile(absPath, data, os.FileMode(pDetails.mode))
			if err != nil {
				log.Printf("[SyncCoordinator] Failed to write inline file: %v\n", err)
				return nil
			}
			
			// Update file_metadata in SQLite DB, preserving version set by HandlePeerMetadataUpdate
			mtime := time.Now().Unix()
			fileVersion := int64(1)
			if existingMeta, _ := sc.db.Metadata().Get(pDetails.repoID, payload.Path); existingMeta != nil {
				fileVersion = existingMeta.Version
			}
			meta := &sqlite.FileMetadata{
				RepositoryID:      pDetails.repoID,
				Filepath:          payload.Path,
				Hash:              computedHash,
				Size:              int64(len(data)),
				Version:           fileVersion,
				LocalLastModified: mtime,
				IsDeleted:         false,
				UpdatedAt:         time.Now().UnixNano() / int64(time.Millisecond),
				Mode:              pDetails.mode,
			}
			_ = sc.db.Metadata().Save(meta)
			log.Printf("[SyncCoordinator] Inline small-file written successfully: %s\n", payload.Path)
			return nil
		}

		// Start download session: connect to the peer's dynamic transferPort
		transferID := fmt.Sprintf("dl_%d_%s", time.Now().UnixNano(), pDetails.repoID)
		err = sc.transferMgr.StartDownload(transferID, payload.Path, pDetails.repoID, peerID, payload.Hash, pDetails.size, host, payload.TransferPort, pDetails.mode)
		if err != nil {
			log.Printf("[SyncCoordinator] StartDownload failed: %v\n", err)
		}
		return nil
	}
	return nil
}

// HandleIPCMessage handles incoming IPC messages from local C++ watcher/daemon.
func (sc *SyncCoordinator) HandleIPCMessage(msg *ipc.Message) error {
	switch msg.Type {
	case "add_repository":
		var payload struct {
			RepoID string `json:"repo_id"`
			Path   string `json:"path"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return err
		}
		return sc.AddRepository(payload.RepoID, payload.Path)

	case "remove_repository":
		var payload struct {
			RepoID string `json:"repo_id"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return err
		}
		return sc.RemoveRepository(payload.RepoID)

	case "file_changed":
		var payload struct {
			RepoID string `json:"repo_id"`
			protocol.FileChangedPayload
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return err
		}
		if err := payload.FileChangedPayload.Validate(); err != nil {
			return fmt.Errorf("invalid file_changed payload: %w", err)
		}
		return sc.HandleLocalFileChanged(payload.RepoID, &payload.FileChangedPayload)
	case "repo_list_request":
		repos, err := sc.db.Repositories().List()
		if err != nil {
			return err
		}
		type RepoInfo struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		}
		list := make([]RepoInfo, 0)
		for _, r := range repos {
			list = append(list, RepoInfo{ID: r.ID, Path: r.LocalPath})
		}
		respPayload, _ := json.Marshal(map[string]interface{}{
			"repos": list,
		})
		resp := &ipc.Message{
			Version:   "1.0",
			Type:      "repo_list_response",
			Source:    "go",
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			Payload:   respPayload,
		}
		sc.ipcServer.SendMessage(resp)
		return nil

	case "repo_status_request":
		var payload struct {
			RepoID string `json:"repo_id"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return err
		}
		files, err := sc.db.Metadata().ListByRepository(payload.RepoID, false)
		if err != nil {
			return err
		}
		type FileInfo struct {
			Path         string `json:"path"`
			Hash         string `json:"hash"`
			Size         int64  `json:"size"`
			Version      int64  `json:"version"`
			ModifiedTime int64  `json:"modified_time"`
		}
		list := make([]FileInfo, 0)
		for _, f := range files {
			list = append(list, FileInfo{
				Path:         f.Filepath,
				Hash:         f.Hash,
				Size:         f.Size,
				Version:      f.Version,
				ModifiedTime: f.LocalLastModified,
			})
		}
		respPayload, _ := json.Marshal(map[string]interface{}{
			"repo_id": payload.RepoID,
			"files":   list,
		})
		resp := &ipc.Message{
			Version:   "1.0",
			Type:      "repo_status_response",
			Source:    "go",
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			Payload:   respPayload,
		}
		sc.ipcServer.SendMessage(resp)
		return nil

	case "conflict_resolution":
		var payload struct {
			RepoID     string `json:"repo_id"`
			Path       string `json:"path"`
			Resolution string `json:"resolution"`
			PeerID     string `json:"peer_id"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return err
		}
		return sc.HandleConflictResolution(payload.RepoID, payload.Path, payload.Resolution, payload.PeerID)

	case "share_repository":
		var payload struct {
			RepoID string `json:"repo_id"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return err
		}
		sc.mu.RLock()
		defer sc.mu.RUnlock()

		files, err := sc.db.Metadata().ListByRepository(payload.RepoID, false)
		if err != nil {
			return fmt.Errorf("list files for share: %w", err)
		}

		conns := sc.connMgr.ActiveConnections()
		for peerID := range conns {
			for _, f := range files {
				sc.sendMetadataUpdateToPeer(peerID, payload.RepoID, f.Filepath, f.Size, uint64(f.Version), f.Hash, f.LocalLastModified, f.IsDeleted, f.Mode)
			}
		}

		resp := &ipc.Message{
			Version:   "1.0",
			Type:      "share_repository_response",
			Source:    "go",
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		}
		respPayload, _ := json.Marshal(map[string]interface{}{
			"repo_id":    payload.RepoID,
			"peer_count": len(conns),
		})
		resp.Payload = respPayload
		sc.ipcServer.SendMessage(resp)
		return nil

	case "join_repository":
		var payload struct {
			RepoID string `json:"repo_id"`
			Path   string `json:"path"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return err
		}
		if err := sc.AddRepository(payload.RepoID, payload.Path); err != nil {
			return fmt.Errorf("add repository for join: %w", err)
		}
		conns := sc.connMgr.ActiveConnections()
		for peerID := range conns {
			sc.SyncAllRepositoriesWithPeer(peerID)
		}
		return nil
	}
	return nil
}

// HandleConflictResolution processes a user's conflict resolution decision
// received from the C++ UI via IPC.
func (sc *SyncCoordinator) HandleConflictResolution(repoID, path, resolution, peerID string) error {
	switch resolution {
	case "local":
		meta, err := sc.db.Metadata().Get(repoID, path)
		if err != nil {
			return fmt.Errorf("get local metadata for conflict resolution: %w", err)
		}
		if meta != nil {
			sc.sendMetadataUpdateToPeer(peerID, repoID, path, meta.Size, uint64(meta.Version), meta.Hash, meta.LocalLastModified, meta.IsDeleted, meta.Mode)
		}
		sc.db.History().LogEvent(&sqlite.SyncEvent{
			EventID:      fmt.Sprintf("conflict_resolved_%d", time.Now().UnixNano()),
			RepositoryID: repoID,
			FilePath:     path,
			PeerID:       peerID,
			Timestamp:    time.Now().Unix(),
			EventType:    "conflict_resolved",
			Status:       "local_kept",
		})
	case "remote":
		_ = sc.db.Metadata().HardDelete(repoID, path)
		sc.db.History().LogEvent(&sqlite.SyncEvent{
			EventID:      fmt.Sprintf("conflict_resolved_%d", time.Now().UnixNano()),
			RepositoryID: repoID,
			FilePath:     path,
			PeerID:       peerID,
			Timestamp:    time.Now().Unix(),
			EventType:    "conflict_resolved",
			Status:       "remote_accepted",
		})
	case "merge":
		sc.db.History().LogEvent(&sqlite.SyncEvent{
			EventID:      fmt.Sprintf("conflict_resolved_%d", time.Now().UnixNano()),
			RepositoryID: repoID,
			FilePath:     path,
			PeerID:       peerID,
			Timestamp:    time.Now().Unix(),
			EventType:    "conflict_resolved",
			Status:       "manual_merge",
		})
	default:
		return fmt.Errorf("unknown conflict resolution: %s", resolution)
	}
	return nil
}

// SyncAllRepositoriesWithPeer sends file_metadata_update messages for all files in all active repos to the given peer.
func (sc *SyncCoordinator) SyncAllRepositoriesWithPeer(peerID string) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	repos, err := sc.db.Repositories().List()
	if err != nil {
		log.Printf("[SyncCoordinator] Error listing repos for sync: %v\n", err)
		return
	}

	for _, repo := range repos {
		if repo.Status != "active" {
			continue
		}
		files, err := sc.db.Metadata().ListByRepository(repo.ID, false)
		if err != nil {
			log.Printf("[SyncCoordinator] Error listing metadata for repo %s: %v\n", repo.ID, err)
			continue
		}

		log.Printf("[SyncCoordinator] Syncing %d files of repo %s with peer %s\n", len(files), repo.ID, peerID)
		for _, f := range files {
			sc.sendMetadataUpdateToPeer(peerID, repo.ID, f.Filepath, f.Size, uint64(f.Version), f.Hash, f.LocalLastModified, f.IsDeleted, f.Mode)
		}
	}
}

func (sc *SyncCoordinator) ScanAndIndexLocalFiles(repoID, localPath string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	scanStartTime := time.Now().UnixMilli()

	log.Printf("[SyncCoordinator] Starting initial directory scan for repository %s at %s\n", repoID, localPath)

	seenFiles := make(map[string]bool)

	err := filepath.WalkDir(localPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and files (like .git, .DS_Store, etc.)
		rel, err := filepath.Rel(localPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if rel != "." {
			parts := strings.Split(rel, "/")
			for _, part := range parts {
				if strings.HasPrefix(part, ".") {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
		}

		if d.IsDir() {
			return nil
		}

		// Skip temporary .tmp files
		if strings.HasSuffix(rel, ".tmp") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		size := info.Size()
		modTime := info.ModTime().UnixMilli()
		mode := uint32(info.Mode())

		seenFiles[rel] = true

		existing, err := sc.db.Metadata().Get(repoID, rel)
		if err != nil {
			log.Printf("[SyncCoordinator] Error getting metadata for %s: %v\n", rel, err)
			return nil
		}

		if existing != nil {
			// Compare size, modified time, and mode to see if it hasn't changed.
			if existing.Size == size && existing.LocalLastModified == modTime && existing.Mode == mode && !existing.IsDeleted {
				// No change, skip hash computation
				return nil
			}

			// Compute hash to see if content actually changed
			hash, err := sc.computeFileSHA256(path)
			if err != nil {
				log.Printf("[SyncCoordinator] Error hashing file %s: %v\n", path, err)
				return nil
			}

			if existing.Hash == hash && existing.Size == size && existing.Mode == mode && !existing.IsDeleted {
				// Only modification time changed, but contents are identical. Update mod time only.
				existing.LocalLastModified = modTime
				if err := sc.db.Metadata().Save(existing); err != nil {
					log.Printf("[SyncCoordinator] Error saving updated metadata for %s: %v\n", rel, err)
				}
				return nil
			}

			// Actual modification offline while app was closed!
			sc.lamportClock.Tick()
			sc.vectorClock.Tick(sc.localPeerID)
			nextVersion := existing.Version + 1

			meta := sqlite.FileMetadata{
				RepositoryID:      repoID,
				Filepath:          rel,
				Hash:              hash,
				Size:              size,
				Version:           nextVersion,
				LocalLastModified: modTime,
				IsDeleted:         false,
				UpdatedAt:         time.Now().UnixMilli(),
				Mode:              mode,
			}

			if err := sc.db.Metadata().Save(&meta); err != nil {
				log.Printf("[SyncCoordinator] Error saving modified metadata for %s: %v\n", rel, err)
				return nil
			}

			// Log to sync history
			histEvent := sqlite.SyncEvent{
				EventID:      fmt.Sprintf("evt_%d_%s", time.Now().UnixNano(), repoID),
				RepositoryID: repoID,
				FilePath:     rel,
				PeerID:       sc.localPeerID,
				Timestamp:    time.Now().Unix(),
				EventType:    "local_change",
				Status:       "success",
			}
			_ = sc.db.History().LogEvent(&histEvent)

			// Broadcast change to other peers
			if size > 0 {
				go sc.broadcastLocalMetadataUpdate(repoID, rel, &meta)
			}
		} else {
			// New file discovered offline!
			hash, err := sc.computeFileSHA256(path)
			if err != nil {
				log.Printf("[SyncCoordinator] Error hashing new file %s: %v\n", path, err)
				return nil
			}

			sc.lamportClock.Tick()
			sc.vectorClock.Tick(sc.localPeerID)

			meta := sqlite.FileMetadata{
				RepositoryID:      repoID,
				Filepath:          rel,
				Hash:              hash,
				Size:              size,
				Version:           1,
				LocalLastModified: modTime,
				IsDeleted:         false,
				UpdatedAt:         time.Now().UnixMilli(),
				Mode:              mode,
			}

			if err := sc.db.Metadata().Save(&meta); err != nil {
				log.Printf("[SyncCoordinator] Error saving new metadata for %s: %v\n", rel, err)
				return nil
			}

			// Log to sync history
			histEvent := sqlite.SyncEvent{
				EventID:      fmt.Sprintf("evt_%d_%s", time.Now().UnixNano(), repoID),
				RepositoryID: repoID,
				FilePath:     rel,
				PeerID:       sc.localPeerID,
				Timestamp:    time.Now().Unix(),
				EventType:    "local_change",
				Status:       "success",
			}
			_ = sc.db.History().LogEvent(&histEvent)

			// Broadcast change to other peers
			if size > 0 {
				go sc.broadcastLocalMetadataUpdate(repoID, rel, &meta)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("directory walk failed: %w", err)
	}

	// Now check for any files in the DB that were deleted offline
	dbFiles, err := sc.db.Metadata().ListByRepository(repoID, false)
	if err != nil {
		return fmt.Errorf("list db files failed: %w", err)
	}

	for _, dbFile := range dbFiles {
		if dbFile.UpdatedAt >= scanStartTime {
			// Skip files that were created or modified during/after the scan started
			continue
		}
		if !seenFiles[dbFile.Filepath] && !dbFile.IsDeleted {
			// This file was deleted offline!
			sc.lamportClock.Tick()
			sc.vectorClock.Tick(sc.localPeerID)
			dbFile.Version = dbFile.Version + 1

			dbFile.IsDeleted = true
			dbFile.UpdatedAt = time.Now().UnixMilli()

			if err := sc.db.Metadata().Save(dbFile); err != nil {
				log.Printf("[SyncCoordinator] Error saving deletion metadata for %s: %v\n", dbFile.Filepath, err)
				continue
			}

			// Log to sync history
			histEvent := sqlite.SyncEvent{
				EventID:      fmt.Sprintf("evt_%d_%s", time.Now().UnixNano(), repoID),
				RepositoryID: repoID,
				FilePath:     dbFile.Filepath,
				PeerID:       sc.localPeerID,
				Timestamp:    time.Now().Unix(),
				EventType:    "local_change",
				Status:       "success",
			}
			_ = sc.db.History().LogEvent(&histEvent)

			// Broadcast deletion update
			go sc.broadcastLocalMetadataUpdate(repoID, dbFile.Filepath, dbFile)
		}
	}

	log.Printf("[SyncCoordinator] Completed initial directory scan for repository %s. Total files tracked: %d\n", repoID, len(seenFiles))
	return nil
}

func (sc *SyncCoordinator) computeFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := sha256.New()
	var buf = make([]byte, 64*1024)
	if _, err := io.CopyBuffer(h, file, buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func (sc *SyncCoordinator) broadcastLocalMetadataUpdate(repoID string, filepath string, meta *sqlite.FileMetadata) {
	p2pMsg := &ipc.Message{
		Version:   "1.0",
		Type:      "file_metadata_update",
		Source:    "go",
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}
	
	sc.mu.RLock()
	vclock := sc.vectorClock.AsMap()
	sc.mu.RUnlock()

	updatePayload := map[string]interface{}{
		"repo_id":       repoID,
		"path":          filepath,
		"hash":          meta.Hash,
		"size":          meta.Size,
		"version":       meta.Version,
		"modified_time": meta.LocalLastModified,
		"is_deleted":    meta.IsDeleted,
		"mode":          meta.Mode,
		"vector_clock":  vclock,
	}
	payloadBytes, err := json.Marshal(updatePayload)
	if err != nil {
		log.Printf("[SyncCoordinator] Error marshalling scan metadata update: %v\n", err)
		return
	}
	p2pMsg.Payload = payloadBytes

	sc.connMgr.Broadcast(p2pMsg)
}
