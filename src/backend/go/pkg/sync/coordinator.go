package sync

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"p2p/pkg/ipc"
	"p2p/pkg/network"
	"p2p/pkg/protocol"
	"p2p/pkg/storage/sqlite"
	"p2p/pkg/transfer"
	"p2p/pkg/versioning"
	"os"
	"os/exec"
	"syscall"
	"path/filepath"
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
	concurrencySem   chan struct{}            // semaphore for concurrent P2P transfers
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
	return &SyncCoordinator{
		db:               db,
		ipcServer:        ipcServer,
		connMgr:          connMgr,
		detector:         versioning.NewConflictDetector(),
		queue:            NewSyncQueue(),
		transferMgr:      transfer.NewFileTransferManager(ipcServer),
		localPeerID:      localPeerID,
		workers:          make(map[string]chan struct{}),
		concurrencySem:   make(chan struct{}, 4), // Max 4 concurrent uploads/downloads
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

		// 2. Find C++ daemon binary
		cppExe := "./cpp_daemon"
		if execPath, errExec := os.Executable(); errExec == nil {
			peerCpp := filepath.Join(filepath.Dir(execPath), "cpp_daemon")
			if _, errStat := os.Stat(peerCpp); errStat == nil {
				cppExe = peerCpp
			}
		}

		if _, err := os.Stat(cppExe); os.IsNotExist(err) {
			candidates := []string{
				"src/backend/cpp/build/bin/cpp_daemon",
				"../cpp/build/bin/cpp_daemon",
				"build/bin/cpp_daemon",
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
				if _, errStat := os.Stat("src/backend/cpp/build/bin/cpp_daemon"); errStat == nil {
					cppExe = "src/backend/cpp/build/bin/cpp_daemon"
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
			// Use process group so we can kill the entire group on shutdown
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
			// Try SIGTERM to the process group first
			pgid, err := syscall.Getpgid(cmd.Process.Pid)
			if err == nil && pgid > 0 {
				syscall.Kill(-pgid, syscall.SIGTERM)
			} else {
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}
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
				if err == nil && pgid > 0 {
					syscall.Kill(-pgid, syscall.SIGKILL)
				} else {
					_ = cmd.Process.Kill()
				}
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

	repo := sqlite.Repository{
		ID:        repoID,
		LocalPath: localPath,
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

	// 1. Fetch current database metadata
	existing, err := sc.db.Metadata().Get(repoID, payload.Path)
	if err == nil && existing != nil {
		// Ignore redundant file change notifications that match the recorded db state (e.g. from sync downloads)
		if existing.Hash == payload.Hash && existing.IsDeleted == (payload.Action == "delete") {
			log.Printf("[SyncCoordinator] Ignoring redundant file change for: %s (hash matches db)\n", payload.Path)
			return nil
		}
	}

	// 2. Tick Lamport clock and vector clock for causal ordering
	nextVersion := sc.lamportClock.Tick()
	sc.vectorClock.Tick(sc.localPeerID)

	// 3. Save new metadata state to SQLite
	meta := sqlite.FileMetadata{
		RepositoryID:      repoID,
		Filepath:          payload.Path,
		Hash:              payload.Hash,
		Size:              payload.Size,
		Version:           int64(nextVersion),
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
		"version":       nextVersion,
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
	return nil
}

// HandlePeerMetadataUpdate processes version changes received from a remote peer.
func (sc *SyncCoordinator) HandlePeerMetadataUpdate(peerID string, repoID string, update map[string]interface{}) {
	path, _ := update["path"].(string)
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

	// Merge remote clocks to preserve causal ordering
	sc.lamportClock.Witness(version)
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

		if resolution.IsConflict {
			sc.logAndNotifyConflict(repoID, path, localFV, remoteFV)
		}

	case versioning.KeepLocal:
		// Remote is behind. Send our updated metadata to them.
		if resolution.IsConflict {
			sc.logAndNotifyConflict(repoID, path, localFV, remoteFV)
		} else {
			sc.sendMetadataUpdateToPeer(peerID, repoID, path, localVer, localHash, localTime, localMeta.IsDeleted)
		}
	case versioning.Skip:
		// Identical hashes, do nothing
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

func (sc *SyncCoordinator) sendMetadataUpdateToPeer(peerID, repoID, path string, version uint64, hash string, modifiedTime int64, isDeleted bool) {
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
		"size":          0,
		"version":       version,
		"modified_time": modifiedTime,
		"is_deleted":    isDeleted,
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

			// Acquire concurrency slot (bandwidth scheduling)
			select {
			case sc.concurrencySem <- struct{}{}:
				// Slot acquired, process sync task
				go func(t *SyncTask) {
					defer func() { <-sc.concurrencySem }()
					sc.executeSyncTask(t)
				}(task)
			case <-sc.stopChan:
				return
			}
		}
	}
}

func (sc *SyncCoordinator) executeSyncTask(task *SyncTask) {
	log.Printf("[SyncCoordinator] Executing sync task: %s %s (%d bytes)\n", task.Type, task.FilePath, task.Size)

	if task.Type == Download {
		if !sc.connMgr.IsConnected(task.PeerID) {
			log.Printf("[SyncCoordinator] Transfer failed: peer %s disconnected\n", task.PeerID)
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
			log.Printf("[SyncCoordinator] Failed to send file_request to peer %s: %v\n", task.PeerID, err)
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
		transferPort, err := sc.transferMgr.StartUpload(transferID, meta.Filepath, peerID, meta.Hash, meta.Size)
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
		respPayload, _ := json.Marshal(protocol.FileResponsePayload{
			Path:         payload.Path,
			Hash:         payload.Hash,
			TransferPort: transferPort,
		})
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

		// Start download session: connect to the peer's dynamic transferPort
		transferID := fmt.Sprintf("dl_%d_%s", time.Now().UnixNano(), pDetails.repoID)
		err = sc.transferMgr.StartDownload(transferID, payload.Path, peerID, payload.Hash, pDetails.size, host, payload.TransferPort, pDetails.mode)
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
	}
	return nil
}
