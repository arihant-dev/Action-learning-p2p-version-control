package transfer

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"p2p/pkg/ipc"
)

// TransferSession represents an active or preparing file transfer session.
type TransferSession struct {
	mu           sync.Mutex
	TransferID   string
	FilePath     string
	PeerID       string
	Type         string // "download" or "upload"
	LocalPort    int
	LocalList    net.Listener
	NetConn      net.Conn // P2P network connection
	Status       string   // "preparing", "streaming", "completed", "failed"
	ExpectedSize int64
	Error        error
}

// FileTransferManager manages local socket handovers and streams raw file data.
type FileTransferManager struct {
	ipcServer *ipc.IpcServer
	sessions  map[string]*TransferSession
	mu        sync.RWMutex
}

// NewFileTransferManager creates a new FileTransferManager.
func NewFileTransferManager(ipcServer *ipc.IpcServer) *FileTransferManager {
	return &FileTransferManager{
		ipcServer: ipcServer,
		sessions:  make(map[string]*TransferSession),
	}
}

// StartDownload initiates a file download. It connects to the peer's transfer port,
// sets up a local listener for the C++ daemon to connect to, and proxies the bytes.
func (ft *FileTransferManager) StartDownload(transferID, filePath, peerID, expectedHash string, expectedSize int64, peerAddr string, peerPort int) error {
	// 1. Connect to remote peer's TCP transfer socket
	addr := net.JoinHostPort(peerAddr, strconv.Itoa(peerPort))
	netConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to remote peer transfer port %s: %w", addr, err)
	}

	// 2. Start local listener for C++ socket handover
	localListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		netConn.Close()
		return fmt.Errorf("failed to listen on local transfer port: %w", err)
	}
	localPort := localListener.Addr().(*net.TCPAddr).Port

	session := &TransferSession{
		TransferID:   transferID,
		FilePath:     filePath,
		PeerID:       peerID,
		Type:         "download",
		LocalPort:    localPort,
		LocalList:    localListener,
		NetConn:      netConn,
		Status:       "preparing",
		ExpectedSize: expectedSize,
	}

	ft.mu.Lock()
	ft.sessions[transferID] = session
	ft.mu.Unlock()

	// 3. Send IPC request to C++ to connect and write the file
	msg := &ipc.Message{
		Version:   "1.0",
		Type:      "prepare_file_transfer",
		Source:    "go",
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}
	payload := map[string]interface{}{
		"transfer_id":   transferID,
		"path":          filePath,
		"peer_id":       peerID,
		"transfer_port": localPort,
		"expected_hash": expectedHash,
		"expected_size": expectedSize,
	}
	payloadBytes, _ := json.Marshal(payload)
	msg.Payload = payloadBytes

	ft.ipcServer.SendMessage(msg)

	// 4. Stream in background
	go ft.streamDownload(session)

	return nil
}

// StartUpload sets up a P2P network port for a remote peer to connect to,
// a local port for C++ to connect to, and streams the data from C++ to the peer.
func (ft *FileTransferManager) StartUpload(transferID, filePath, peerID, expectedHash string, expectedSize int64) (int, error) {
	// 1. Listen for P2P remote peer connection
	netListener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("failed to listen on P2P network port: %w", err)
	}
	netPort := netListener.Addr().(*net.TCPAddr).Port

	// 2. Listen for local C++ connection
	localListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		netListener.Close()
		return 0, fmt.Errorf("failed to listen on local transfer port: %w", err)
	}
	localPort := localListener.Addr().(*net.TCPAddr).Port

	session := &TransferSession{
		TransferID:   transferID,
		FilePath:     filePath,
		PeerID:       peerID,
		Type:         "upload",
		LocalPort:    localPort,
		LocalList:    localListener,
		Status:       "preparing",
		ExpectedSize: expectedSize,
	}

	ft.mu.Lock()
	ft.sessions[transferID] = session
	ft.mu.Unlock()

	// 3. Send IPC request to C++ to connect and stream/read the file
	msg := &ipc.Message{
		Version:   "1.0",
		Type:      "prepare_file_transfer",
		Source:    "go",
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}
	payload := map[string]interface{}{
		"transfer_id":   transferID,
		"path":          filePath,
		"peer_id":       peerID,
		"transfer_port": localPort,
		"expected_hash": expectedHash,
		"expected_size": expectedSize,
	}
	payloadBytes, _ := json.Marshal(payload)
	msg.Payload = payloadBytes

	ft.ipcServer.SendMessage(msg)

	// 4. Accept connections and stream in background
	go ft.streamUpload(session, netListener)

	return netPort, nil
}

func (ft *FileTransferManager) streamDownload(session *TransferSession) {
	defer session.NetConn.Close()
	defer session.LocalList.Close()

	if l, ok := session.LocalList.(*net.TCPListener); ok {
		_ = l.SetDeadline(time.Now().Add(10 * time.Second))
	}
	localConn, err := session.LocalList.Accept()
	if err != nil {
		log.Printf("[FileTransferManager] C++ failed to connect to local download socket: %v\n", err)
		ft.finishSession(session, fmt.Errorf("C++ connect timeout: %w", err))
		return
	}
	defer localConn.Close()

	session.mu.Lock()
	session.Status = "streaming"
	session.mu.Unlock()

	// Set deadlines to avoid socket leakage on network hangs
	_ = session.NetConn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_ = localConn.SetWriteDeadline(time.Now().Add(30 * time.Second))

	// Copy from network peer connection to local C++ connection
	limitReader := io.LimitReader(session.NetConn, session.ExpectedSize)
	copied, err := io.Copy(localConn, limitReader)
	if err != nil {
		log.Printf("[FileTransferManager] Download streaming error: %v\n", err)
		ft.finishSession(session, err)
		return
	}

	if copied != session.ExpectedSize {
		log.Printf("[FileTransferManager] Download size mismatch: got %d, expected %d\n", copied, session.ExpectedSize)
		ft.finishSession(session, fmt.Errorf("size mismatch: got %d, expected %d", copied, session.ExpectedSize))
		return
	}

	ft.finishSession(session, nil)
}

func (ft *FileTransferManager) streamUpload(session *TransferSession, netListener net.Listener) {
	defer netListener.Close()
	defer session.LocalList.Close()

	if l, ok := netListener.(*net.TCPListener); ok {
		_ = l.SetDeadline(time.Now().Add(10 * time.Second))
	}
	netConn, err := netListener.Accept()
	if err != nil {
		log.Printf("[FileTransferManager] Remote peer failed to connect for upload: %v\n", err)
		ft.finishSession(session, fmt.Errorf("remote peer connect timeout: %w", err))
		return
	}
	session.NetConn = netConn
	defer netConn.Close()

	if l, ok := session.LocalList.(*net.TCPListener); ok {
		_ = l.SetDeadline(time.Now().Add(10 * time.Second))
	}
	localConn, err := session.LocalList.Accept()
	if err != nil {
		log.Printf("[FileTransferManager] C++ failed to connect to local upload socket: %v\n", err)
		ft.finishSession(session, fmt.Errorf("C++ connect timeout: %w", err))
		return
	}
	defer localConn.Close()

	session.mu.Lock()
	session.Status = "streaming"
	session.mu.Unlock()

	// Set deadlines to avoid socket leakage on network hangs
	_ = localConn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_ = session.NetConn.SetWriteDeadline(time.Now().Add(30 * time.Second))

	// Copy from local C++ connection to network peer connection
	limitReader := io.LimitReader(localConn, session.ExpectedSize)
	copied, err := io.Copy(session.NetConn, limitReader)
	if err != nil {
		log.Printf("[FileTransferManager] Upload streaming error: %v\n", err)
		ft.finishSession(session, err)
		return
	}

	if copied != session.ExpectedSize {
		log.Printf("[FileTransferManager] Upload size mismatch: got %d, expected %d\n", copied, session.ExpectedSize)
		ft.finishSession(session, fmt.Errorf("size mismatch: got %d, expected %d", copied, session.ExpectedSize))
		return
	}

	ft.finishSession(session, nil)
}

func (ft *FileTransferManager) finishSession(session *TransferSession, err error) {
	session.mu.Lock()
	session.Error = err
	if err != nil {
		session.Status = "failed"
	} else {
		session.Status = "completed"
	}
	session.mu.Unlock()

	completeMsg := &ipc.Message{
		Version:   "1.0",
		Type:      "file_transfer_complete",
		Source:    "go",
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}

	success := err == nil
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	payload := map[string]interface{}{
		"transfer_id": session.TransferID,
		"path":        session.FilePath,
		"success":     success,
		"error":       errStr,
	}
	payloadBytes, _ := json.Marshal(payload)
	completeMsg.Payload = payloadBytes

	ft.ipcServer.SendMessage(completeMsg)
}

// GetSession returns a copy of a session state.
func (ft *FileTransferManager) GetSession(transferID string) (TransferSession, bool) {
	ft.mu.RLock()
	s, exists := ft.sessions[transferID]
	ft.mu.RUnlock()
	if !exists {
		return TransferSession{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return TransferSession{
		TransferID:   s.TransferID,
		FilePath:     s.FilePath,
		PeerID:       s.PeerID,
		Type:         s.Type,
		LocalPort:    s.LocalPort,
		Status:       s.Status,
		ExpectedSize: s.ExpectedSize,
		Error:        s.Error,
	}, true
}

// GetSessions returns a copy of all active/completed transfer sessions.
func (ft *FileTransferManager) GetSessions() []TransferSession {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	list := make([]TransferSession, 0, len(ft.sessions))
	for _, s := range ft.sessions {
		s.mu.Lock()
		list = append(list, TransferSession{
			TransferID:   s.TransferID,
			FilePath:     s.FilePath,
			PeerID:       s.PeerID,
			Type:         s.Type,
			LocalPort:    s.LocalPort,
			Status:       s.Status,
			ExpectedSize: s.ExpectedSize,
			Error:        s.Error,
		})
		s.mu.Unlock()
	}
	return list
}
