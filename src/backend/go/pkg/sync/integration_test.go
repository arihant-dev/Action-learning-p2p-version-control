package sync

import (
	"net"
	"testing"
	"time"

	"p2p/pkg/ipc"
	"p2p/pkg/network"
	"p2p/pkg/protocol"
	"p2p/pkg/storage/sqlite"
)

// TestLamportClockPropagation verifies that when PeerA creates a file, the
// Lamport clock is ticked locally and the remote peer's clock is Witness'd
// upon receiving the metadata update — ensuring causal ordering.
func TestLamportClockPropagation(t *testing.T) {
	dbA, dbB := testDBs(t)

	sockA, sockB, cleanup := testIPCServers(t)
	defer cleanup()

	connMgrA, connMgrB := testConnectedPeers(t)
	defer connMgrA.Stop()
	defer connMgrB.Stop()

	coordA := NewSyncCoordinator(dbA, sockA, connMgrA, "PeerA")
	coordB := NewSyncCoordinator(dbB, sockB, connMgrB, "PeerB")
	mustStart(t, coordA, coordB)

	connMgrA.OnMessage = func(peerID string, msg *ipc.Message) {
		_ = coordA.HandleP2PMessage(peerID, msg)
	}
	connMgrB.OnMessage = func(peerID string, msg *ipc.Message) {
		_ = coordB.HandleP2PMessage(peerID, msg)
	}

	mustAddRepo(t, coordA, "shared-repo", "/tmp/a")
	mustAddRepo(t, coordB, "shared-repo", "/tmp/b")

	// PeerA creates a file — ticks Lamport clock to 1
	changePayload := &protocol.FileChangedPayload{
		Action:       "add",
		Path:         "doc.txt",
		Hash:         "hash-v1",
		Size:         100,
		ModifiedTime: time.Now().Unix(),
	}
	if err := coordA.HandleLocalFileChanged("shared-repo", changePayload); err != nil {
		t.Fatalf("handle local change: %v", err)
	}

	// PeerA's clock should be 1
	if v := coordA.lamportClock.Value(); v != 1 {
		t.Errorf("expected PeerA Lamport clock = 1, got %d", v)
	}

	// Wait for propagation to PeerB
	time.Sleep(300 * time.Millisecond)

	// PeerB's clock should have been Witness'd to 1 (then ticked to 2 via Witness())
	if v := coordB.lamportClock.Value(); v < 1 {
		t.Errorf("expected PeerB Lamport clock >= 1 (Witness'd), got %d", v)
	}

	// PeerB should have the metadata
	metaB, err := dbB.Metadata().Get("shared-repo", "doc.txt")
	if err != nil {
		t.Fatalf("fetch metadata B: %v", err)
	}
	if metaB.Hash != "hash-v1" || metaB.Version != 1 {
		t.Errorf("expected hash-v1 version 1 on B, got hash=%s version=%d", metaB.Hash, metaB.Version)
	}

	// PeerA creates another file — ticks to 2
	changePayload2 := &protocol.FileChangedPayload{
		Action:       "add",
		Path:         "doc2.txt",
		Hash:         "hash-v2",
		Size:         200,
		ModifiedTime: time.Now().Unix(),
	}
	if err := coordA.HandleLocalFileChanged("shared-repo", changePayload2); err != nil {
		t.Fatalf("handle second change: %v", err)
	}

	if v := coordA.lamportClock.Value(); v != 2 {
		t.Errorf("expected PeerA Lamport clock = 2 after second change, got %d", v)
	}

	time.Sleep(200 * time.Millisecond)

	// PeerB's clock should have been Witness'd to at least the received version
	if v := coordB.lamportClock.Value(); v < 2 {
		t.Errorf("expected PeerB Lamport clock >= 2, got %d", v)
	}
}

// TestConcurrentEditDetected verifies that two peers editing the same file
// concurrently (same Lamport version, different hash) results in a conflict
// being flagged by the ConflictDetector, and an IPC notification is sent.
func TestConcurrentEditDetected(t *testing.T) {
	dbA, dbB := testDBs(t)

	sockA, sockB, cleanup := testIPCServers(t)
	defer cleanup()

	// Start dummy IPC consumers to prevent blocked sends
	go consumeIPC(sockA)
	go consumeIPC(sockB)

	connMgrA, connMgrB := testConnectedPeers(t)
	defer connMgrA.Stop()
	defer connMgrB.Stop()

	coordA := NewSyncCoordinator(dbA, sockA, connMgrA, "PeerA")
	coordB := NewSyncCoordinator(dbB, sockB, connMgrB, "PeerB")
	mustStart(t, coordA, coordB)

	connMgrA.OnMessage = func(peerID string, msg *ipc.Message) {
		_ = coordA.HandleP2PMessage(peerID, msg)
	}
	connMgrB.OnMessage = func(peerID string, msg *ipc.Message) {
		_ = coordB.HandleP2PMessage(peerID, msg)
	}

	mustAddRepo(t, coordA, "shared-repo", "/tmp/a")
	mustAddRepo(t, coordB, "shared-repo", "/tmp/b")

	// PeerA creates a file (version 1)
	changePayload := &protocol.FileChangedPayload{
		Action:       "add",
		Path:         "shared.txt",
		Hash:         "hash-A",
		Size:         100,
		ModifiedTime: 5000,
	}
	if err := coordA.HandleLocalFileChanged("shared-repo", changePayload); err != nil {
		t.Fatalf("handle local change A: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// PeerB receives version 1. Now simulate concurrent edit:
	// Both have version 1, but B edits with a different hash and later timestamp.
	updatePayload := map[string]interface{}{
		"path":          "shared.txt",
		"hash":          "hash-B",
		"size":          200.0,
		"version":       1.0,
		"modified_time": 6000.0,
		"is_deleted":    false,
		"vector_clock":  map[string]interface{}{"PeerB": 1.0},
	}

	// This should trigger a conflict since version=1 matches local version=1
	// but hash differs and remote timestamp (6000) > local (5000)
	coordB.HandlePeerMetadataUpdate("PeerA", "shared-repo", updatePayload)

	// Check that the conflict was logged
	events, err := dbB.History().GetByRepository("shared-repo")
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	foundConflict := false
	for _, e := range events {
		if e.EventType == "conflict_detected" && e.FilePath == "shared.txt" {
			foundConflict = true
			break
		}
	}
	if !foundConflict {
		t.Error("expected conflict_detected event in B's history")
	}
}

// TestMetadataUpdatePushOnKeepLocal verifies that when a remote peer has an
// older version than the local peer, the coordinator triggers a metadata
// update push back to the remote peer (via sendMetadataUpdateToPeer), and
// the remote peer receives and applies the newer metadata.
func TestMetadataUpdatePushOnKeepLocal(t *testing.T) {
	dbA, dbB := testDBs(t)

	sockA, sockB, cleanup := testIPCServers(t)
	defer cleanup()
	go consumeIPC(sockA)
	go consumeIPC(sockB)

	connMgrA, connMgrB := testConnectedPeers(t)
	defer connMgrA.Stop()
	defer connMgrB.Stop()

	coordA := NewSyncCoordinator(dbA, sockA, connMgrA, "PeerA")
	coordB := NewSyncCoordinator(dbB, sockB, connMgrB, "PeerB")
	mustStart(t, coordA, coordB)

	connMgrA.OnMessage = func(peerID string, msg *ipc.Message) {
		_ = coordA.HandleP2PMessage(peerID, msg)
	}
	connMgrB.OnMessage = func(peerID string, msg *ipc.Message) {
		_ = coordB.HandleP2PMessage(peerID, msg)
	}

	mustAddRepo(t, coordA, "shared-repo", "/tmp/a")
	mustAddRepo(t, coordB, "shared-repo", "/tmp/b")

	// Pre-populate PeerA with version 5 (newer)
	meta := sqlite.FileMetadata{
		RepositoryID:      "shared-repo",
		Filepath:          "notes.txt",
		Hash:              "hash-v5",
		Size:              500,
		Version:           5,
		LocalLastModified: time.Now().Unix(),
		IsDeleted:         false,
		UpdatedAt:         time.Now().Unix(),
	}
	if err := dbA.Metadata().Save(&meta); err != nil {
		t.Fatalf("save meta A: %v", err)
	}
	// Manually tick clocks to match version 5
	for i := 0; i < 5; i++ {
		coordA.lamportClock.Tick()
		coordA.vectorClock.Tick("PeerA")
	}

	// PeerA receives an older version (version 3) from PeerB
	oldUpdate := map[string]interface{}{
		"path":          "notes.txt",
		"hash":          "hash-v3",
		"size":          300.0,
		"version":       3.0,
		"modified_time": float64(time.Now().Unix() - 100),
		"is_deleted":    false,
		"vector_clock":  map[string]interface{}{"PeerB": 3.0},
	}

	coordA.HandlePeerMetadataUpdate("PeerB", "shared-repo", oldUpdate)

	// A should determine remote is behind (KeepLocal) and push metadata update
	// back to B via the network connection. B's OnMessage should process it.
	time.Sleep(300 * time.Millisecond)

	// A's local state should be unchanged (KeepLocal)
	localMeta, err := dbA.Metadata().Get("shared-repo", "notes.txt")
	if err != nil {
		t.Fatalf("fetch meta A: %v", err)
	}
	if localMeta.Version != 5 {
		t.Errorf("expected local version to remain 5, got %d", localMeta.Version)
	}
	if localMeta.Hash != "hash-v5" {
		t.Errorf("expected local hash hash-v5, got %s", localMeta.Hash)
	}

	// PeerB should have received the pushed update and applied it
	bMeta, err := dbB.Metadata().Get("shared-repo", "notes.txt")
	if err != nil {
		t.Fatalf("fetch meta B: %v", err)
	}
	if bMeta == nil {
		t.Fatal("expected PeerB to have metadata after push, got nil")
	}
	if bMeta.Hash != "hash-v5" {
		t.Errorf("expected PeerB hash hash-v5, got %s", bMeta.Hash)
	}
}

// TestVectorClockMergeOnPeerUpdate verifies that when PeerA receives a
// metadata update from PeerB containing a vector clock, PeerA's vector
// clock is properly merged (max of each component).
func TestVectorClockMergeOnPeerUpdate(t *testing.T) {
	dbA, _ := testDBs(t)

	sockA, _, cleanup := testIPCServers(t)
	defer cleanup()

	connMgrA := network.NewConnectionManager("PeerA")
	if err := connMgrA.StartServer(0); err != nil {
		t.Fatalf("start server A: %v", err)
	}
	defer connMgrA.Stop()

	coordA := NewSyncCoordinator(dbA, sockA, connMgrA, "PeerA")
	mustStart(t, coordA)
	mustAddRepo(t, coordA, "repo", "/tmp/r")

	// Pre-populate A's metadata and tick its vector clock
	meta := sqlite.FileMetadata{
		RepositoryID:      "repo",
		Filepath:          "f.txt",
		Hash:              "hash-local",
		Size:              100,
		Version:           3,
		LocalLastModified: time.Now().Unix(),
		IsDeleted:         false,
		UpdatedAt:         time.Now().Unix(),
	}
	if err := dbA.Metadata().Save(&meta); err != nil {
		t.Fatalf("save meta: %v", err)
	}
	// Tick A's clock to version 3
	for i := 0; i < 3; i++ {
		coordA.lamportClock.Tick()
		coordA.vectorClock.Tick("PeerA")
	}

	// A receives update from PeerB with vector clock {PeerB: 5, PeerA: 1}
	updatePayload := map[string]interface{}{
		"path":          "f.txt",
		"hash":          "hash-remote",
		"size":          200.0,
		"version":       5.0,
		"modified_time": float64(time.Now().Unix()),
		"is_deleted":    false,
		"vector_clock":  map[string]interface{}{"PeerA": 1.0, "PeerB": 5.0},
	}
	coordA.HandlePeerMetadataUpdate("PeerB", "repo", updatePayload)

	// A's vector clock should have merged: PeerA = max(3, 1) = 3, PeerB = max(0, 5) = 5
	mergedVC := coordA.vectorClock.AsMap()
	if mergedVC["PeerA"] != 3 {
		t.Errorf("expected PeerA clock = 3 (max of local 3 and remote 1), got %d", mergedVC["PeerA"])
	}
	if mergedVC["PeerB"] != 5 {
		t.Errorf("expected PeerB clock = 5 (max of local 0 and remote 5), got %d", mergedVC["PeerB"])
	}
}

// TestSkipOnIdenticalHash verifies the Skip resolution when local and remote
// have identical hashes — no action is taken and no IPC message is sent.
func TestSkipOnIdenticalHash(t *testing.T) {
	dbA, _ := testDBs(t)

	sockA, _, cleanup := testIPCServers(t)
	defer cleanup()

	connMgrA := network.NewConnectionManager("PeerA")
	if err := connMgrA.StartServer(0); err != nil {
		t.Fatalf("start server A: %v", err)
	}
	defer connMgrA.Stop()

	coordA := NewSyncCoordinator(dbA, sockA, connMgrA, "PeerA")
	mustStart(t, coordA)
	mustAddRepo(t, coordA, "repo", "/tmp/r")

	meta := sqlite.FileMetadata{
		RepositoryID:      "repo",
		Filepath:          "same.txt",
		Hash:              "identical-hash",
		Size:              100,
		Version:           1,
		LocalLastModified: time.Now().Unix(),
		IsDeleted:         false,
		UpdatedAt:         time.Now().Unix(),
	}
	if err := dbA.Metadata().Save(&meta); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	// Same hash — should resolve to Skip
	updatePayload := map[string]interface{}{
		"path":          "same.txt",
		"hash":          "identical-hash",
		"size":          100.0,
		"version":       2.0,
		"modified_time": float64(time.Now().Unix()),
		"is_deleted":    false,
		"vector_clock":  map[string]interface{}{"PeerB": 2.0},
	}
	coordA.HandlePeerMetadataUpdate("PeerB", "repo", updatePayload)

	// Local state should remain unchanged (Skip)
	localMeta, err := dbA.Metadata().Get("repo", "same.txt")
	if err != nil {
		t.Fatalf("fetch meta: %v", err)
	}
	if localMeta.Version != 1 {
		t.Errorf("expected version to remain 1 on Skip, got %d", localMeta.Version)
	}
}

// --- Helpers ---

func testDBs(t *testing.T) (*sqlite.DB, *sqlite.DB) {
	t.Helper()
	dbA, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open db A: %v", err)
	}
	t.Cleanup(func() { dbA.Close() })

	dbB, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open db B: %v", err)
	}
	t.Cleanup(func() { dbB.Close() })

	return dbA, dbB
}

func testIPCServers(t *testing.T) (*ipc.IpcServer, *ipc.IpcServer, func()) {
	t.Helper()
	// Use unique socket paths per test
	sockA := "/tmp/test_int_" + t.Name() + "_A.sock"
	sockB := "/tmp/test_int_" + t.Name() + "_B.sock"

	ipcA := ipc.NewIpcServer(sockA)
	if err := ipcA.Start(); err != nil {
		t.Fatalf("ipc A: %v", err)
	}

	ipcB := ipc.NewIpcServer(sockB)
	if err := ipcB.Start(); err != nil {
		ipcA.Stop()
		t.Fatalf("ipc B: %v", err)
	}

	cleanup := func() {
		ipcA.Stop()
		ipcB.Stop()
	}
	return ipcA, ipcB, cleanup
}

func testConnectedPeers(t *testing.T) (*network.ConnectionManager, *network.ConnectionManager) {
	t.Helper()
	cmA := network.NewConnectionManager("PeerA")
	if err := cmA.StartServer(0); err != nil {
		t.Fatalf("server A: %v", err)
	}

	cmB := network.NewConnectionManager("PeerB")
	if err := cmB.StartServer(0); err != nil {
		cmA.Stop()
		t.Fatalf("server B: %v", err)
	}

	// Connect A -> B
	if err := cmA.Connect("PeerB", "127.0.0.1", cmB.Port()); err != nil {
		cmA.Stop()
		cmB.Stop()
		t.Fatalf("connect A->B: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	return cmA, cmB
}

func mustStart(t *testing.T, coords ...*SyncCoordinator) {
	t.Helper()
	for _, c := range coords {
		if err := c.Start(); err != nil {
			t.Fatalf("start coordinator: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, c := range coords {
			c.Stop()
		}
	})
}

func mustAddRepo(t *testing.T, coord *SyncCoordinator, repoID, path string) {
	t.Helper()
	if err := coord.AddRepository(repoID, path); err != nil {
		t.Fatalf("add repo %s: %v", repoID, err)
	}
}

func consumeIPC(sock *ipc.IpcServer) {
	conn, err := net.Dial("unix", sock.SocketPath())
	if err != nil {
		return
	}
	defer conn.Close()
	for {
		_, err := ipc.ReadMessage(conn)
		if err != nil {
			return
		}
	}
}
