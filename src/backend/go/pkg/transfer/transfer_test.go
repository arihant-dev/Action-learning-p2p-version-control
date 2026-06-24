package transfer

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"p2p/pkg/ipc"
)

func TestFileTransferSessionCreation(t *testing.T) {
	sockPath := "/tmp/test_transfer_session.sock"
	defer os.Remove(sockPath)

	ipcServer := ipc.NewIpcServer(sockPath)
	err := ipcServer.Start()
	if err != nil {
		t.Fatalf("start ipc server: %v", err)
	}
	defer ipcServer.Stop()

	ft := NewFileTransferManager(ipcServer)

	// Mock peer connection
	netListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer netListener.Close()
	netPort := netListener.Addr().(*net.TCPAddr).Port

	// Start P2P listener in bg
	go func() {
		conn, err := netListener.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	err = ft.StartDownload("trans_123", "docs/file.txt", "peer_bob", "hash123", 100, "127.0.0.1", netPort)
	if err != nil {
		t.Fatalf("start download failed: %v", err)
	}

	session, exists := ft.GetSession("trans_123")
	if !exists {
		t.Fatal("session not found")
	}
	if session.Status != "preparing" && session.Status != "streaming" {
		t.Errorf("unexpected session status: %s", session.Status)
	}
	if session.FilePath != "docs/file.txt" {
		t.Errorf("unexpected filepath: %s", session.FilePath)
	}
}

func TestFileTransferStreaming(t *testing.T) {
	sockPath := "/tmp/test_transfer_streaming.sock"
	defer os.Remove(sockPath)

	ipcServer := ipc.NewIpcServer(sockPath)
	err := ipcServer.Start()
	if err != nil {
		t.Fatalf("start ipc: %v", err)
	}
	defer ipcServer.Stop()

	// Connect mock C++ client to receive prepare messages
	var prepareMsgReceived chan *ipc.Message = make(chan *ipc.Message, 5)
	ipcServer.OnMessage = func(msg *ipc.Message) error {
		return nil
	}

	// We need a dummy client to connect to the Unix/TCP socket so messages are consumed.
	var client net.Conn
	if ipcServer.ToC != nil {
		// Connect client
		client, err = net.Dial("unix", sockPath)
		if err != nil {
			client, err = net.Dial("tcp", "127.0.0.1:9999")
		}
		if err == nil {
			defer client.Close()
			go func() {
				for {
					msg, err := ipc.ReadMessage(client)
					if err != nil {
						return
					}
					if msg.Type == "prepare_file_transfer" {
						prepareMsgReceived <- msg
					}
				}
			}()
		}
	}

	ft := NewFileTransferManager(ipcServer)

	// Start P2P server that uploads mock content
	p2pListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer p2pListener.Close()
	p2pPort := p2pListener.Addr().(*net.TCPAddr).Port

	mockData := "Hello action learning peer-to-peer!"
	go func() {
		conn, err := p2pListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, mockData)
	}()

	err = ft.StartDownload("dl_session_1", "notes.txt", "peer_alice", "hashxyz", int64(len(mockData)), "127.0.0.1", p2pPort)
	if err != nil {
		t.Fatalf("start download: %v", err)
	}

	// Wait for prepare message to get local port
	var prepareMsg *ipc.Message
	select {
	case prepareMsg = <-prepareMsgReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for prepare message")
	}

	var payload map[string]interface{}
	_ = json.Unmarshal(prepareMsg.Payload, &payload)
	localPort := int(payload["transfer_port"].(float64))

	// Connect to localPort as C++ mock and read
	localConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		t.Fatalf("C++ failed to connect to local port %d: %v", localPort, err)
	}
	defer localConn.Close()

	buf := make([]byte, 100)
	n, err := localConn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read failed: %v", err)
	}

	got := string(buf[:n])
	if got != mockData {
		t.Errorf("got %q, want %q", got, mockData)
	}

	// Verify session status is completed
	time.Sleep(100 * time.Millisecond)
	session, _ := ft.GetSession("dl_session_1")
	if session.Status != "completed" {
		t.Errorf("expected session completed, got %s", session.Status)
	}
}

func TestIPv6AddressHandling(t *testing.T) {
	sockPath := "/tmp/test_ipv6.sock"
	defer os.Remove(sockPath)

	ipcServer := ipc.NewIpcServer(sockPath)
	err := ipcServer.Start()
	if err != nil {
		t.Fatalf("start ipc: %v", err)
	}
	defer ipcServer.Stop()

	ft := NewFileTransferManager(ipcServer)

	// Set up P2P listener on IPv6 loopback
	netListener, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 loopback not supported on this host")
	}
	defer netListener.Close()
	netPort := netListener.Addr().(*net.TCPAddr).Port

	// Start P2P listener accept loop in bg
	go func() {
		conn, err := netListener.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	err = ft.StartDownload("trans_ipv6", "docs/file.txt", "peer_bob", "hash123", 100, "::1", netPort)
	if err != nil {
		t.Fatalf("StartDownload failed with IPv6: %v", err)
	}

	session, exists := ft.GetSession("trans_ipv6")
	if !exists {
		t.Fatal("session not found")
	}
	if session.ExpectedSize != 100 {
		t.Errorf("expected size 100, got %d", session.ExpectedSize)
	}
}

func TestFileTransferSizeMismatch(t *testing.T) {
	sockPath := "/tmp/test_transfer_mismatch.sock"
	defer os.Remove(sockPath)

	ipcServer := ipc.NewIpcServer(sockPath)
	err := ipcServer.Start()
	if err != nil {
		t.Fatalf("start ipc: %v", err)
	}
	defer ipcServer.Stop()

	ft := NewFileTransferManager(ipcServer)

	p2pListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer p2pListener.Close()
	p2pPort := p2pListener.Addr().(*net.TCPAddr).Port

	mockData := "Hello" // 5 bytes
	go func() {
		conn, err := p2pListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, mockData)
	}()

	// We expect 10 bytes, but mockData is only 5 bytes.
	err = ft.StartDownload("session_mismatch", "notes.txt", "peer_alice", "hashxyz", 10, "127.0.0.1", p2pPort)
	if err != nil {
		t.Fatalf("start download: %v", err)
	}

	// We need to trigger the accept on the local listener so the streaming starts and fails.
	time.Sleep(50 * time.Millisecond)
	session, exists := ft.GetSession("session_mismatch")
	if !exists {
		t.Fatal("session not found")
	}

	localConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", session.LocalPort))
	if err != nil {
		t.Fatalf("C++ failed to connect: %v", err)
	}
	defer localConn.Close()

	// Read what's available
	buf := make([]byte, 20)
	_, _ = localConn.Read(buf)

	time.Sleep(100 * time.Millisecond)
	session, _ = ft.GetSession("session_mismatch")
	if session.Status != "failed" {
		t.Errorf("expected session failed due to size mismatch, got %s", session.Status)
	}
	if session.Error == nil {
		t.Error("expected error to be populated, got nil")
	}
}

func TestFileTransferUploadStreaming(t *testing.T) {
	sockPath := "/tmp/test_transfer_upload.sock"
	defer os.Remove(sockPath)

	ipcServer := ipc.NewIpcServer(sockPath)
	err := ipcServer.Start()
	if err != nil {
		t.Fatalf("start ipc: %v", err)
	}
	defer ipcServer.Stop()

	// Connect mock C++ client so message channel functions
	cConn, err := net.Dial("unix", sockPath)
	if err != nil {
		cConn, err = net.Dial("tcp", "127.0.0.1:9999")
	}
	if err == nil {
		defer cConn.Close()
		go func() {
			for {
				_, _ = ipc.ReadMessage(cConn)
			}
		}()
	}

	ft := NewFileTransferManager(ipcServer)

	mockData := "Upload file content"
	netPort, err := ft.StartUpload("up_session_1", "docs/file.txt", "peer_bob", "hash123", int64(len(mockData)))
	if err != nil {
		t.Fatalf("StartUpload failed: %v", err)
	}

	// 1. Peer dials to the returned P2P netPort
	peerConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", netPort))
	if err != nil {
		t.Fatalf("peer failed to dial upload port: %v", err)
	}
	defer peerConn.Close()

	// 2. C++ daemon dials the local session transfer port and streams data
	session, exists := ft.GetSession("up_session_1")
	if !exists {
		t.Fatal("session not found")
	}

	localConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", session.LocalPort))
	if err != nil {
		t.Fatalf("C++ failed to dial local port: %v", err)
	}
	defer localConn.Close()

	// C++ writes mock data
	_, err = io.WriteString(localConn, mockData)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Read peerConn to check received payload
	buf := make([]byte, 100)
	n, err := peerConn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("failed to read from peer socket: %v", err)
	}

	got := string(buf[:n])
	if got != mockData {
		t.Errorf("expected %q, got %q", mockData, got)
	}

	// Wait and verify session completion
	time.Sleep(100 * time.Millisecond)
	session, _ = ft.GetSession("up_session_1")
	if session.Status != "completed" {
		t.Errorf("expected session completed, got %s", session.Status)
	}
}

func TestFileTransferUploadSizeMismatch(t *testing.T) {
	sockPath := "/tmp/test_transfer_upload_mismatch.sock"
	defer os.Remove(sockPath)

	ipcServer := ipc.NewIpcServer(sockPath)
	err := ipcServer.Start()
	if err != nil {
		t.Fatalf("start ipc: %v", err)
	}
	defer ipcServer.Stop()

	// Connect mock C++ client
	cConn, err := net.Dial("unix", sockPath)
	if err != nil {
		cConn, err = net.Dial("tcp", "127.0.0.1:9999")
	}
	if err == nil {
		defer cConn.Close()
		go func() {
			for {
				_, _ = ipc.ReadMessage(cConn)
			}
		}()
	}

	ft := NewFileTransferManager(ipcServer)

	// Expect 20 bytes, but C++ will write only 5 bytes
	netPort, err := ft.StartUpload("up_session_mismatch", "docs/file.txt", "peer_bob", "hash123", 20)
	if err != nil {
		t.Fatalf("StartUpload failed: %v", err)
	}

	peerConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", netPort))
	if err != nil {
		t.Fatalf("peer failed to dial upload port: %v", err)
	}
	defer peerConn.Close()

	session, exists := ft.GetSession("up_session_mismatch")
	if !exists {
		t.Fatal("session not found")
	}

	localConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", session.LocalPort))
	if err != nil {
		t.Fatalf("C++ failed to dial local port: %v", err)
	}
	defer localConn.Close()

	_, _ = io.WriteString(localConn, "Short")
	localConn.Close()

	// Read peerConn to trigger EOF
	buf := make([]byte, 100)
	_, _ = peerConn.Read(buf)

	time.Sleep(100 * time.Millisecond)
	session, _ = ft.GetSession("up_session_mismatch")
	if session.Status != "failed" {
		t.Errorf("expected session failed, got %s", session.Status)
	}
	if session.Error == nil {
		t.Error("expected error, got nil")
	}
}
