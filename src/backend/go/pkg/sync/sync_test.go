package sync

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"p2p/pkg/ipc"
	"p2p/pkg/network"
	"p2p/pkg/protocol"
	"p2p/pkg/storage/sqlite"
)

func TestQueuePrioritization(t *testing.T) {
	q := NewSyncQueue()

	// Push 3 tasks for RepoA with different sizes
	q.Push(&SyncTask{RepoID: "RepoA", FilePath: "large.mp4", Type: Download, Size: 5000, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "RepoA", FilePath: "small.txt", Type: Download, Size: 10, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "RepoA", FilePath: "medium.pdf", Type: Download, Size: 100, Timestamp: time.Now()})

	// Popping should return small.txt first, then medium.pdf, then large.mp4
	t1 := q.Pop()
	if t1.FilePath != "small.txt" {
		t.Errorf("expected small.txt, got %s", t1.FilePath)
	}

	t2 := q.Pop()
	if t2.FilePath != "medium.pdf" {
		t.Errorf("expected medium.pdf, got %s", t2.FilePath)
	}

	t3 := q.Pop()
	if t3.FilePath != "large.mp4" {
		t.Errorf("expected large.mp4, got %s", t3.FilePath)
	}
}

func TestQueueFairScheduling(t *testing.T) {
	q := NewSyncQueue()

	// Push tasks for multiple repositories
	q.Push(&SyncTask{RepoID: "RepoA", FilePath: "a1.txt", Type: Download, Size: 10, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "RepoA", FilePath: "a2.txt", Type: Download, Size: 20, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "RepoB", FilePath: "b1.txt", Type: Download, Size: 5, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "RepoC", FilePath: "c1.txt", Type: Download, Size: 15, Timestamp: time.Now()})

	// Round-robin pops across RepoA, RepoB, RepoC
	// First pop should get a task from RepoA (first pushed)
	t1 := q.Pop()
	if t1.RepoID != "RepoA" {
		t.Errorf("pop 1: expected RepoA, got %s", t1.RepoID)
	}

	// Second pop should get a task from RepoB
	t2 := q.Pop()
	if t2.RepoID != "RepoB" {
		t.Errorf("pop 2: expected RepoB, got %s", t2.RepoID)
	}

	// Third pop should get a task from RepoC
	t3 := q.Pop()
	if t3.RepoID != "RepoC" {
		t.Errorf("pop 3: expected RepoC, got %s", t3.RepoID)
	}

	// Fourth pop should get the remaining task from RepoA
	t4 := q.Pop()
	if t4.RepoID != "RepoA" || t4.FilePath != "a2.txt" {
		t.Errorf("pop 4: expected RepoA/a2.txt, got %s/%s", t4.RepoID, t4.FilePath)
	}

	// Fifth pop should be empty
	t5 := q.Pop()
	if t5 != nil {
		t.Errorf("expected empty queue, got task: %v", t5)
	}
}

func TestCoordinatorLifecycle(t *testing.T) {
	// Create fresh in-memory database
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ipcServer := ipc.NewIpcServer("/tmp/test_sync_coordinator.sock")
	connMgr := network.NewConnectionManager("peer_local")

	coord := NewSyncCoordinator(db, ipcServer, connMgr, "peer_local")

	err = coord.Start()
	if err != nil {
		t.Fatalf("coordinator start failed: %v", err)
	}
	defer coord.Stop()

	// Add repository
	err = coord.AddRepository("repo_1", "/home/user/repo1")
	if err != nil {
		t.Fatalf("add repository: %v", err)
	}

	// Verify repo saved in DB
	repo, err := db.Repositories().Get("repo_1")
	if err != nil {
		t.Errorf("expected repo in DB, got error: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(repo.LocalPath), "repo1") {
		t.Errorf("unexpected local path: %s", repo.LocalPath)
	}

	// Remove repository
	err = coord.RemoveRepository("repo_1")
	if err != nil {
		t.Fatalf("remove repository: %v", err)
	}

	repoDeleted, err := db.Repositories().Get("repo_1")
	if err != nil {
		t.Fatalf("failed to fetch deleted repository: %v", err)
	}
	if repoDeleted != nil {
		t.Error("expected repo to be deleted from DB, but it exists")
	}
}

func TestLocalFileChanged(t *testing.T) {
	// 1. Setup DB
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// 2. Setup mock IPC
	sockPath := "/tmp/test_ipc_file_changed.sock"
	defer os.Remove(sockPath)
	ipcServer := ipc.NewIpcServer(sockPath)
	err = ipcServer.Start()
	if err != nil {
		t.Fatalf("start IPC: %v", err)
	}
	defer ipcServer.Stop()

	// 3. Setup coordinator
	connMgr := network.NewConnectionManager("peer_local")
	coord := NewSyncCoordinator(db, ipcServer, connMgr, "peer_local")
	_ = coord.Start()
	defer coord.Stop()

	_ = coord.AddRepository("repo_abc", "/tmp/sync")

	// 4. Trigger change
	changePayload := &protocol.FileChangedPayload{
		Action:       "add",
		Path:         "notes.txt",
		Hash:         "hash123",
		Size:         500,
		ModifiedTime: time.Now().Unix(),
	}

	err = coord.HandleLocalFileChanged("repo_abc", changePayload)
	if err != nil {
		t.Fatalf("handle local change: %v", err)
	}

	// 5. Verify saved in DB
	meta, err := db.Metadata().Get("repo_abc", "notes.txt")
	if err != nil {
		t.Fatalf("fetch metadata: %v", err)
	}
	if meta.Hash != "hash123" || meta.Version != 1 {
		t.Errorf("invalid metadata saved: %+v", meta)
	}
}

func TestPeerMetadataUpdateTombstone(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ipcServer := ipc.NewIpcServer("/tmp/test_ipc_tombstone.sock")
	connMgr := network.NewConnectionManager("peer_local")
	coord := NewSyncCoordinator(db, ipcServer, connMgr, "peer_local")
	_ = coord.Start()
	defer coord.Stop()

	_ = coord.AddRepository("repo_xyz", "/tmp/sync")

	// Save active metadata in local DB
	meta := sqlite.FileMetadata{
		RepositoryID:      "repo_xyz",
		Filepath:          "notes.txt",
		Hash:              "hash123",
		Size:              500,
		Version:           2,
		LocalLastModified: time.Now().Unix(),
		IsDeleted:         false,
		UpdatedAt:         time.Now().Unix(),
	}
	_ = db.Metadata().Save(&meta)

	// Simulate peer deletion metadata update with higher version (version = 5, is_deleted = true)
	updatePayload := map[string]interface{}{
		"path":          "notes.txt",
		"hash":          "hash_deleted",
		"size":          0.0,
		"version":       5.0,
		"modified_time": float64(time.Now().Unix()),
		"is_deleted":    true,
	}

	coord.HandlePeerMetadataUpdate("peer_remote", "repo_xyz", updatePayload)

	// Verify local state has updated to deleted/tombstone
	updatedMeta, err := db.Metadata().Get("repo_xyz", "notes.txt")
	if err != nil {
		t.Fatalf("fetch metadata: %v", err)
	}
	if !updatedMeta.IsDeleted {
		t.Error("expected local file to be marked as deleted")
	}
	if updatedMeta.Version != 5 {
		t.Errorf("expected version 5, got %d", updatedMeta.Version)
	}
}

func TestSyncLoopPrevention(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ipcServer := ipc.NewIpcServer("/tmp/test_ipc_loop.sock")
	connMgr := network.NewConnectionManager("peer_local")
	coord := NewSyncCoordinator(db, ipcServer, connMgr, "peer_local")
	_ = coord.Start()
	defer coord.Stop()

	_ = coord.AddRepository("repo_xyz", "/tmp/sync")

	// Save active metadata in local DB with version = 5
	meta := sqlite.FileMetadata{
		RepositoryID:      "repo_xyz",
		Filepath:          "notes.txt",
		Hash:              "hash123",
		Size:              500,
		Version:           5,
		LocalLastModified: time.Now().Unix(),
		IsDeleted:         false,
		UpdatedAt:         time.Now().Unix(),
	}
	_ = db.Metadata().Save(&meta)

	// Receive peer update with older version (version = 3)
	updatePayload := map[string]interface{}{
		"path":          "notes.txt",
		"hash":          "hash_old",
		"size":          200.0,
		"version":       3.0,
		"modified_time": float64(time.Now().Unix() - 100),
		"is_deleted":    false,
	}

	coord.HandlePeerMetadataUpdate("peer_remote", "repo_xyz", updatePayload)

	// Local version should remain 5 and NOT be updated or ticked
	localMeta, err := db.Metadata().Get("repo_xyz", "notes.txt")
	if err != nil {
		t.Fatalf("fetch metadata: %v", err)
	}
	if localMeta.Version != 5 || localMeta.Hash != "hash123" {
		t.Errorf("expected version 5 and hash123, got version %d and hash %s", localMeta.Version, localMeta.Hash)
	}
}

func TestEndToEndP2PMetadataAndFileTransfer(t *testing.T) {
	// 1. Setup two databases
	dbA, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("db A: %v", err)
	}
	defer dbA.Close()

	dbB, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("db B: %v", err)
	}
	defer dbB.Close()

	// 2. Setup mock IPC socket paths
	sockA := "/tmp/test_sync_e2e_A.sock"
	sockB := "/tmp/test_sync_e2e_B.sock"
	defer os.Remove(sockA)
	defer os.Remove(sockB)

	ipcServerA := ipc.NewIpcServer(sockA)
	if err := ipcServerA.Start(); err != nil {
		t.Fatalf("ipc A: %v", err)
	}
	defer ipcServerA.Stop()

	ipcServerB := ipc.NewIpcServer(sockB)
	if err := ipcServerB.Start(); err != nil {
		t.Fatalf("ipc B: %v", err)
	}
	defer ipcServerB.Stop()

	// Connect dummy clients so toC channels are consumed
	cliA, _ := net.Dial("unix", sockA)
	if cliA != nil {
		defer cliA.Close()
		go func() {
			for {
				_, _ = ipc.ReadMessage(cliA)
			}
		}()
	}
	cliB, _ := net.Dial("unix", sockB)
	if cliB != nil {
		defer cliB.Close()
		go func() {
			for {
				_, _ = ipc.ReadMessage(cliB)
			}
		}()
	}

	// 3. Setup Connection Managers
	connMgrA := network.NewConnectionManager("PeerA")
	if err := connMgrA.StartServer(0); err != nil { // dynamic port
		t.Fatalf("p2p server A: %v", err)
	}
	defer connMgrA.Stop()

	connMgrB := network.NewConnectionManager("PeerB")
	if err := connMgrB.StartServer(0); err != nil { // dynamic port
		t.Fatalf("p2p server B: %v", err)
	}
	defer connMgrB.Stop()

	// Connect Peer A to Peer B
	portB := connMgrB.Port()
	err = connMgrA.Connect("PeerB", "127.0.0.1", portB)
	if err != nil {
		t.Fatalf("failed to connect PeerA to PeerB: %v", err)
	}

	// Wait for handshake
	time.Sleep(50 * time.Millisecond)

	// 4. Create SyncCoordinators
	coordA := NewSyncCoordinator(dbA, ipcServerA, connMgrA, "PeerA")
	if err := coordA.Start(); err != nil {
		t.Fatalf("start coord A: %v", err)
	}
	defer coordA.Stop()

	coordB := NewSyncCoordinator(dbB, ipcServerB, connMgrB, "PeerB")
	if err := coordB.Start(); err != nil {
		t.Fatalf("start coord B: %v", err)
	}
	defer coordB.Stop()

	// Route incoming messages
	connMgrA.OnMessage = func(peerID string, msg *ipc.Message) {
		_ = coordA.HandleP2PMessage(peerID, msg)
	}
	connMgrB.OnMessage = func(peerID string, msg *ipc.Message) {
		_ = coordB.HandleP2PMessage(peerID, msg)
	}

	// Add repository on both sides
	err = coordA.AddRepository("shared_repo", "/tmp/syncA")
	if err != nil {
		t.Fatalf("add repo A: %v", err)
	}
	err = coordB.AddRepository("shared_repo", "/tmp/syncB")
	if err != nil {
		t.Fatalf("add repo B: %v", err)
	}

	// 5. Trigger local file change on Peer A
	changeTime := time.Now().Unix()
	fileContent := "Hello P2P Action Learning!"
	changePayload := &protocol.FileChangedPayload{
		Action:       "add",
		Path:         "docs/readme.txt",
		Hash:         "hash12345",
		Size:         int64(len(fileContent)),
		ModifiedTime: changeTime,
	}

	err = coordA.HandleLocalFileChanged("shared_repo", changePayload)
	if err != nil {
		t.Fatalf("local change A: %v", err)
	}

	// Wait for metadata propagation, scheduler queue popping, and the file request handshake
	time.Sleep(300 * time.Millisecond)

	// Peer B should have a download session
	sessionsB := coordB.transferMgr.GetSessions()
	if len(sessionsB) != 1 {
		t.Fatalf("expected 1 session on Peer B, got %d", len(sessionsB))
	}
	sessB := &sessionsB[0]
	if sessB.Type != "download" || sessB.FilePath != "docs/readme.txt" {
		t.Fatalf("invalid session on Peer B: %+v", sessB)
	}

	// Peer A should have an upload session
	sessionsA := coordA.transferMgr.GetSessions()
	if len(sessionsA) != 1 {
		t.Fatalf("expected 1 session on Peer A, got %d", len(sessionsA))
	}
	sessA := &sessionsA[0]
	if sessA.Type != "upload" || sessA.FilePath != "docs/readme.txt" {
		t.Fatalf("invalid session on Peer A: %+v", sessA)
	}

	// Now we simulate C++ daemons connecting to local ports and transferring raw data!
	// Peer B's mock C++ daemon starts reading from its local session port.
	doneChan := make(chan string)
	go func() {
		localConnB, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", sessB.LocalPort))
		if err != nil {
			t.Errorf("B: C++ failed to dial local port: %v", err)
			close(doneChan)
			return
		}
		defer localConnB.Close()

		buf := make([]byte, 100)
		n, err := localConnB.Read(buf)
		if err != nil && err != io.EOF {
			t.Errorf("B: read failed: %v", err)
			close(doneChan)
			return
		}
		doneChan <- string(buf[:n])
	}()

	// Peer A's mock C++ daemon starts writing to its local session port.
	localConnA, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", sessA.LocalPort))
	if err != nil {
		t.Fatalf("A: C++ failed to dial local port: %v", err)
	}
	defer localConnA.Close()

	_, err = io.WriteString(localConnA, fileContent)
	if err != nil {
		t.Fatalf("A: write failed: %v", err)
	}

	// Wait for Peer B to receive the content
	select {
	case gotContent := <-doneChan:
		if gotContent != fileContent {
			t.Errorf("expected content %q, got %q", fileContent, gotContent)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for end-to-end file streaming")
	}

	// Verify both sessions complete successfully
	time.Sleep(50 * time.Millisecond)
	sessionA, _ := coordA.transferMgr.GetSession(sessA.TransferID)
	sessionB, _ := coordB.transferMgr.GetSession(sessB.TransferID)

	if sessionA.Status != "completed" {
		t.Errorf("expected Peer A upload session completed, got %s (error: %v)", sessionA.Status, sessionA.Error)
	}
	if sessionB.Status != "completed" {
		t.Errorf("expected Peer B download session completed, got %s (error: %v)", sessionB.Status, sessionB.Error)
	}
}
