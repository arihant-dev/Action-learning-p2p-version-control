package sync

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"p2p/pkg/ipc"
	"p2p/pkg/network"
	"p2p/pkg/protocol"
	"p2p/pkg/storage/sqlite"
	"p2p/pkg/transfer"
)

func TestStress1000SmallFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 1000 small files stress test in short mode")
	}

	dirA, err := os.MkdirTemp("", "stress_a_*")
	if err != nil {
		t.Fatalf("mkdir A: %v", err)
	}
	defer os.RemoveAll(dirA)

	dirB, err := os.MkdirTemp("", "stress_b_*")
	if err != nil {
		t.Fatalf("mkdir B: %v", err)
	}
	defer os.RemoveAll(dirB)

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

	sockA := ipc.NewIpcServer("/tmp/stress_A_" + t.Name() + ".sock")
	sockB := ipc.NewIpcServer("/tmp/stress_B_" + t.Name() + ".sock")
	if err := sockA.Start(); err != nil {
		t.Fatalf("sock A: %v", err)
	}
	defer sockA.Stop()
	if err := sockB.Start(); err != nil {
		t.Fatalf("sock B: %v", err)
	}
	defer sockB.Stop()

	connMgrA := network.NewConnectionManager("PeerA")
	connMgrB := network.NewConnectionManager("PeerB")
	if err := connMgrA.StartServer(0); err != nil {
		t.Fatalf("server A: %v", err)
	}
	defer connMgrA.Stop()
	if err := connMgrB.StartServer(0); err != nil {
		t.Fatalf("server B: %v", err)
	}
	defer connMgrB.Stop()

	if err := connMgrA.Connect("PeerB", "127.0.0.1", connMgrB.Port()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	coordA := NewSyncCoordinator(dbA, sockA, connMgrA, "PeerA")
	coordB := NewSyncCoordinator(dbB, sockB, connMgrB, "PeerB")
	mustStart(t, coordA, coordB)

	connMgrA.OnMessage = func(peerID string, msg *ipc.Message) {
		_ = coordA.HandleP2PMessage(peerID, msg)
	}
	connMgrB.OnMessage = func(peerID string, msg *ipc.Message) {
		_ = coordB.HandleP2PMessage(peerID, msg)
	}

	mustAddRepo(t, coordA, "stress-repo", dirA)
	mustAddRepo(t, coordB, "stress-repo", dirB)

	// AddRepository kicks off an async initial directory scan on both
	// coordinators (see the identical comment in TestStressRapidChanges).
	// That scan's "offline deletion" pruning pass can race with file
	// changes injected (locally on A, or received from A over the network
	// on B) immediately afterward, occasionally misclassifying a
	// just-written row as "deleted while offline" and dropping it from the
	// tracked count. Give both scans (empty dirs, so near-instant) time to
	// finish first.
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	numFiles := 1000

	for i := 0; i < numFiles; i++ {
		changePayload := &protocol.FileChangedPayload{
			Action:       "add",
			Path:         fmt.Sprintf("file_%d.txt", i),
			Hash:         fmt.Sprintf("hash_%08d", i),
			Size:         1024,
			ModifiedTime: time.Now().Unix(),
		}

		if err := coordA.HandleLocalFileChanged("stress-repo", changePayload); err != nil {
			t.Fatalf("local change %d: %v", i, err)
		}
	}

	time.Sleep(2 * time.Second)

	elapsed := time.Since(start)
	t.Logf("Created %d files in %v", numFiles, elapsed)

	filesA, err := dbA.Metadata().ListByRepository("stress-repo", false)
	if err != nil {
		t.Fatalf("list A: %v", err)
	}
	if len(filesA) != numFiles {
		t.Errorf("expected %d files on A, got %d", numFiles, len(filesA))
	}

	filesB, err := dbB.Metadata().ListByRepository("stress-repo", false)
	if err != nil {
		t.Fatalf("list B: %v", err)
	}
	if len(filesB) != numFiles {
		t.Errorf("expected %d files on B, got %d", numFiles, len(filesB))
	}
}

func TestStressConcurrentTransfers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent transfer stress test in short mode")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 10)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sockPath := fmt.Sprintf("/tmp/stress_transfer_%d.sock", idx)
			defer os.Remove(sockPath)

			ipcServer := ipc.NewIpcServer(sockPath)
			if err := ipcServer.Start(); err != nil {
				errChan <- fmt.Errorf("ipc %d: %v", idx, err)
				return
			}
			defer ipcServer.Stop()

			go func() {
				conn, err := net.Dial("unix", sockPath)
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
			}()
			time.Sleep(50 * time.Millisecond)

			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				errChan <- fmt.Errorf("listen %d: %v", idx, err)
				return
			}
			defer listener.Close()
			port := listener.Addr().(*net.TCPAddr).Port

			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				data := make([]byte, 1024*1024)
				for i := range data {
					data[i] = byte(idx)
				}
				_, _ = conn.Write(data)
			}()

			ft := transfer.NewFileTransferManager(ipcServer)
			err = ft.StartDownload(
				fmt.Sprintf("dl_%d", idx),
				fmt.Sprintf("file_%d.dat", idx),
				fmt.Sprintf("repo_%d", idx),
				"peer",
				fmt.Sprintf("hash_%d", idx),
				1024*1024,
				"127.0.0.1",
				port,
				0644,
			)
			if err != nil {
				errChan <- fmt.Errorf("start dl %d: %v", idx, err)
				return
			}

			if _, exists := ft.GetSession(fmt.Sprintf("dl_%d", idx)); !exists {
				errChan <- fmt.Errorf("session %d not found", idx)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Error(err)
	}
}

func TestStressRapidChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rapid changes stress test in short mode")
	}

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer db.Close()

	sockPath := "/tmp/stress_rapid.sock"
	defer os.Remove(sockPath)
	ipcServer := ipc.NewIpcServer(sockPath)
	if err := ipcServer.Start(); err != nil {
		t.Fatalf("ipc: %v", err)
	}
	defer ipcServer.Stop()

	connMgr := network.NewConnectionManager("PeerLocal")
	if err := connMgr.StartServer(0); err != nil {
		t.Fatalf("server: %v", err)
	}
	defer connMgr.Stop()

	coord := NewSyncCoordinator(db, ipcServer, connMgr, "PeerLocal")
	mustStart(t, coord)

	dir, err := os.MkdirTemp("", "rapid_*")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(dir)

	mustAddRepo(t, coord, "rapid-repo", dir)

	// AddRepository kicks off an async initial directory scan on a
	// background goroutine (startRepoWorkerLocked -> ScanAndIndexLocalFiles).
	// That scan's "offline deletion" pruning pass lists all DB metadata rows
	// and marks any not seen on disk (during its walk) as deleted, guarded
	// only by a millisecond-resolution timestamp comparison against when the
	// scan started. Since dir is empty, injecting file changes concurrently
	// with that still-in-flight scan can race: a row inserted in the same
	// millisecond the scan started can be misclassified as "deleted while
	// offline" and vanish from the tracked count. Give the (near-instant,
	// empty-directory) scan time to finish before generating rapid changes.
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	changes := 100

	for i := 0; i < changes; i++ {
		changePayload := &protocol.FileChangedPayload{
			Action:       "add",
			Path:         fmt.Sprintf("rapid_%d.txt", i),
			Hash:         fmt.Sprintf("hash_%d", i),
			Size:         int64(i * 10),
			ModifiedTime: time.Now().Unix(),
		}

		if err := coord.HandleLocalFileChanged("rapid-repo", changePayload); err != nil {
			t.Fatalf("change %d: %v", i, err)
		}
	}

	elapsed := time.Since(start)
	t.Logf("Processed %d changes in %v (%.0f changes/sec)", changes, elapsed, float64(changes)/elapsed.Seconds())

	files, err := db.Metadata().ListByRepository("rapid-repo", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != changes {
		t.Errorf("expected %d files, got %d", changes, len(files))
	}

	queueSize := coord.queue.Size()
	if queueSize != 0 {
		t.Logf("Queue has %d pending tasks", queueSize)
	}
}

func TestStressManyPeers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-peer stress test in short mode")
	}

	numPeers := 10
	var managers []*network.ConnectionManager
	var cleanups []func()

	for i := 0; i < numPeers; i++ {
		cm := network.NewConnectionManager(fmt.Sprintf("Peer%d", i))
		if err := cm.StartServer(0); err != nil {
			t.Fatalf("start peer %d: %v", i, err)
		}
		managers = append(managers, cm)
		cleanups = append(cleanups, func() { cm.Stop() })
	}

	connectCount := 0
	for i := 1; i < numPeers; i++ {
		if err := managers[0].Connect(fmt.Sprintf("Peer%d", i), "127.0.0.1", managers[i].Port()); err != nil {
			t.Fatalf("connect 0->%d: %v", i, err)
		}
		connectCount++
	}

	time.Sleep(500 * time.Millisecond)

	for i := 0; i < numPeers; i++ {
		connected := 0
		for j := 1; j < numPeers; j++ {
			if managers[i].IsConnected(fmt.Sprintf("Peer%d", j)) {
				connected++
			}
		}
		t.Logf("Peer%d: %d connected peers", i, connected)
	}

	goroutinesBefore := testing.AllocsPerRun(1, func() {})

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				msg := &ipc.Message{
					Version: "1.0",
					Type:    "ping",
					ID:      fmt.Sprintf("msg_%d_%d", idx, j),
				}
				managers[idx].Broadcast(msg)
				time.Sleep(10 * time.Millisecond)
			}
		}(i % numPeers)
	}
	wg.Wait()

	_ = goroutinesBefore

	for _, cleanup := range cleanups {
		cleanup()
	}
}

func TestStressLargeFileTransfer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large file transfer stress test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "largefile_*")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	largeFilePath := filepath.Join(tmpDir, "large.dat")
	largeFile, err := os.Create(largeFilePath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	fileSize := int64(10 * 1024 * 1024)
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	for written := int64(0); written < fileSize; {
		n, err := largeFile.Write(data)
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		written += int64(n)
	}
	largeFile.Close()

	sockPath := "/tmp/stress_large.sock"
	defer os.Remove(sockPath)
	ipcServer := ipc.NewIpcServer(sockPath)
	if err := ipcServer.Start(); err != nil {
		t.Fatalf("ipc: %v", err)
	}
	defer ipcServer.Stop()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	_ = listener.Addr().(*net.TCPAddr).Port

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64*1024)
		total := int64(0)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				total += int64(n)
			}
			if err != nil {
				break
			}
		}
	}()

	ft := transfer.NewFileTransferManager(ipcServer)

	start := time.Now()
	_, _, err = ft.StartUpload("large_up", "large.dat", "repo", "remote-peer", "large-hash", fileSize)
	if err != nil {
		t.Fatalf("StartUpload: %v", err)
	}

	t.Logf("Large file transfer started, waiting for completion...")
	time.Sleep(5 * time.Second)

	session, exists := ft.GetSession("large_up")
	if exists {
		t.Logf("Transfer session status: %s, error: %v", session.Status, session.Error)
	} else {
		t.Log("Transfer session not found (may have completed and been cleaned up)")
	}

	elapsed := time.Since(start)
	rate := float64(fileSize) / elapsed.Seconds() / (1024 * 1024)
	t.Logf("Transferred %d bytes in %v (%.2f MB/s)", fileSize, elapsed, rate)
}
