package transfer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"p2p/pkg/ipc"
	"p2p/pkg/log"
)

type TransferSession struct {
	mu           sync.Mutex
	TransferID   string
	FilePath     string
	PeerID       string
	Type         string
	LocalPort    int
	LocalList    net.Listener
	NetConn      net.Conn
	Status       string
	ExpectedSize int64
	Error        error
}

type FileTransferManager struct {
	ipcServer *ipc.IpcServer
	sessions  map[string]*TransferSession
	mu        sync.RWMutex
}

func NewFileTransferManager(ipcServer *ipc.IpcServer) *FileTransferManager {
	return &FileTransferManager{
		ipcServer: ipcServer,
		sessions:  make(map[string]*TransferSession),
	}
}

func (ft *FileTransferManager) StartDownload(transferID, filePath, repoID, peerID, expectedHash string, expectedSize int64, peerAddr string, peerPort int, mode uint32) error {
	netConn, err := dialWithFallback(peerAddr, peerPort, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to remote peer transfer port %s:%d: %w", peerAddr, peerPort, err)
	}

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

	msg := &ipc.Message{
		Version:   "1.0",
		Type:      "prepare_file_transfer",
		Source:    "go",
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}
	payload := map[string]interface{}{
		"transfer_id":   transferID,
		"path":          filePath,
		"repo_id":       repoID,
		"peer_id":       peerID,
		"transfer_port": localPort,
		"expected_hash": expectedHash,
		"expected_size": expectedSize,
		"direction":     "download",
		"mode":          mode,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		netConn.Close()
		localListener.Close()
		ft.mu.Lock()
		delete(ft.sessions, transferID)
		ft.mu.Unlock()
		return fmt.Errorf("marshal download payload: %w", err)
	}
	msg.Payload = payloadBytes

	ft.ipcServer.SendMessage(msg)

	go ft.streamDownload(session)

	return nil
}

func (ft *FileTransferManager) StartUpload(transferID, filePath, basePath, repoID, peerID, expectedHash string, expectedSize int64) (int, string, error) {
	if expectedSize <= 1024 {
		readPath := filePath
		if basePath != "" {
			readPath = filepath.Join(basePath, filePath)
		}
		data, err := readFileForUpload(readPath)
		if err == nil {
			return 0, data, nil
		}
	}

	netListener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, "", fmt.Errorf("failed to listen on P2P network port: %w", err)
	}
	netPort := netListener.Addr().(*net.TCPAddr).Port

	localListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		netListener.Close()
		return 0, "", fmt.Errorf("failed to listen on local transfer port: %w", err)
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

	msg := &ipc.Message{
		Version:   "1.0",
		Type:      "prepare_file_transfer",
		Source:    "go",
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}
	payload := map[string]interface{}{
		"transfer_id":   transferID,
		"path":          filePath,
		"repo_id":       repoID,
		"peer_id":       peerID,
		"transfer_port": localPort,
		"expected_hash": expectedHash,
		"expected_size": expectedSize,
		"direction":     "upload",
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		netListener.Close()
		localListener.Close()
		ft.mu.Lock()
		delete(ft.sessions, transferID)
		ft.mu.Unlock()
		return 0, "", fmt.Errorf("marshal upload payload: %w", err)
	}
	msg.Payload = payloadBytes

	ft.ipcServer.SendMessage(msg)

	go ft.streamUpload(session, netListener)

	return netPort, "", nil
}

func (ft *FileTransferManager) streamDownload(session *TransferSession) {
	defer session.NetConn.Close()
	defer session.LocalList.Close()

	if l, ok := session.LocalList.(*net.TCPListener); ok {
		_ = l.SetDeadline(time.Now().Add(10 * time.Second))
	}
	localConn, err := session.LocalList.Accept()
	if err != nil {
		log.NewLogger("FileTransferManager").Error().Err(err).Msg("C++ failed to connect to local download socket")
		ft.finishSession(session, fmt.Errorf("C++ connect timeout: %w", err))
		return
	}
	defer localConn.Close()

	session.mu.Lock()
	session.Status = "streaming"
	session.mu.Unlock()

	timeout := 30 * time.Second
	if session.ExpectedSize > 0 {
		perMB := 10 * time.Second
		needed := time.Duration(session.ExpectedSize/(1024*1024)) * perMB
		if needed > timeout {
			timeout = needed
		}
	}
	_ = session.NetConn.SetReadDeadline(time.Now().Add(timeout))
	_ = localConn.SetWriteDeadline(time.Now().Add(timeout))

	limitReader := io.LimitReader(session.NetConn, session.ExpectedSize)
	copied, err := io.Copy(localConn, limitReader)
	if err != nil {
		log.NewLogger("FileTransferManager").Error().Err(err).Msg("Download streaming error")
		ft.finishSession(session, err)
		return
	}

	if copied != session.ExpectedSize {
		log.NewLogger("FileTransferManager").Warn().Int64("got", copied).Int64("expected", session.ExpectedSize).Msg("Download size mismatch")
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
		log.NewLogger("FileTransferManager").Error().Err(err).Msg("Remote peer failed to connect for upload")
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
		log.NewLogger("FileTransferManager").Error().Err(err).Msg("C++ failed to connect to local upload socket")
		ft.finishSession(session, fmt.Errorf("C++ connect timeout: %w", err))
		return
	}
	defer localConn.Close()

	session.mu.Lock()
	session.Status = "streaming"
	session.mu.Unlock()

	timeout := 30 * time.Second
	if session.ExpectedSize > 0 {
		perMB := 10 * time.Second
		needed := time.Duration(session.ExpectedSize/(1024*1024)) * perMB
		if needed > timeout {
			timeout = needed
		}
	}
	_ = localConn.SetReadDeadline(time.Now().Add(timeout))
	_ = session.NetConn.SetWriteDeadline(time.Now().Add(timeout))

	limitReader := io.LimitReader(localConn, session.ExpectedSize)
	copied, err := io.Copy(session.NetConn, limitReader)
	if err != nil {
		log.NewLogger("FileTransferManager").Error().Err(err).Msg("Upload streaming error")
		ft.finishSession(session, err)
		return
	}

	if copied != session.ExpectedSize {
		log.NewLogger("FileTransferManager").Warn().Int64("got", copied).Int64("expected", session.ExpectedSize).Msg("Upload size mismatch")
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

	ft.mu.Lock()
	if len(ft.sessions) > 1000 {
		for id, s := range ft.sessions {
			s.mu.Lock()
			if s.Status == "completed" || s.Status == "failed" {
				delete(ft.sessions, id)
			}
			s.mu.Unlock()
		}
	}
	ft.mu.Unlock()

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
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.NewLogger("FileTransferManager").Error().Err(err).Msg("Failed to marshal completion payload")
	} else {
		completeMsg.Payload = payloadBytes
		ft.ipcServer.SendMessage(completeMsg)
	}
}

func (ft *FileTransferManager) CleanupStaleSessions() {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	for id, s := range ft.sessions {
		s.mu.Lock()
		if s.Status == "completed" || s.Status == "failed" {
			delete(ft.sessions, id)
		}
		s.mu.Unlock()
	}
}

func readFileForUpload(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file for inline transfer: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

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

func dialWithFallback(address string, port int, timeout time.Duration) (net.Conn, error) {
	addr := net.JoinHostPort(address, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err == nil {
		return conn, nil
	}

	// Check if this is an IPv6 link-local address (starts with "fe80:")
	ip := net.ParseIP(address)
	if ip != nil && ip.To4() == nil && strings.HasPrefix(strings.ToLower(address), "fe80:") {
		// 1. Try common fallback zone interfaces
		interfaces, intErr := net.Interfaces()
		if intErr == nil {
			for _, iface := range interfaces {
				if (iface.Flags & net.FlagUp) != 0 {
					zonedAddr := net.JoinHostPort(address+"%"+iface.Name, strconv.Itoa(port))
					log.NewLogger("FileTransferManager").Warn().Msg("Trying fallback zoned address: "+zonedAddr)
					c, e := net.DialTimeout("tcp", zonedAddr, 2*time.Second)
					if e == nil {
						return c, nil
					}
				}
			}
		}

		// 2. Also try localhost fallbacks in case the peer is running locally (e.g. in loopback integration tests)
		localhosts := []string{"127.0.0.1", "::1"}
		for _, lhost := range localhosts {
			laddr := net.JoinHostPort(lhost, strconv.Itoa(port))
			log.NewLogger("FileTransferManager").Warn().Msg("Trying local fallback address: "+laddr)
			c, e := net.DialTimeout("tcp", laddr, 1*time.Second)
			if e == nil {
				return c, nil
			}
		}
	}

	return nil, err
}
