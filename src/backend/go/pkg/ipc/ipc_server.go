package ipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
)

type Message struct {
	Version   string          `json:"version"`
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Timestamp int64           `json:"timestamp"`
	Source    string          `json:"source"`
	Payload   json.RawMessage `json:"payload"`
}

type IpcServer struct {
	socketPath string
	listener   net.Listener
	clients    map[net.Conn]*sync.Mutex
	clientMu   sync.Mutex

	// Callback for handling messages from C++ daemon
	OnMessage func(*Message) error

	// Channel for sending messages to C++ daemon
	ToC chan *Message

	// Protects against send-on-closed-channel during shutdown
	stopChan chan struct{}
	stopOnce sync.Once

	// Buffer the latest state messages to replay to late-connecting clients
	latestMessages map[string]*Message
	latestMu       sync.RWMutex
}

func NewIpcServer(socketPath string) *IpcServer {
	return &IpcServer{
		socketPath:     socketPath,
		clients:        make(map[net.Conn]*sync.Mutex),
		ToC:            make(chan *Message, 100),
		stopChan:       make(chan struct{}),
		latestMessages: make(map[string]*Message),
	}
}

func (s *IpcServer) SocketPath() string {
	return s.socketPath
}

func (s *IpcServer) Start() error {
	var listener net.Listener
	var err error

	// Remove existing socket file if it exists
	os.Remove(s.socketPath)

	// Try Unix socket first (Linux/macOS)
	listener, err = net.Listen("unix", s.socketPath)
	if err != nil {
		// Fallback to TCP on Windows or if Unix socket fails
		fmt.Println("Unix socket not available, falling back to TCP")
		port := deriveFallbackPort(s.socketPath)
		listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return err
		}
	}

	s.listener = listener
	fmt.Printf("IPC server listening on: %s\n", s.socketPath)

	// Set Unix socket permissions if applicable
	if _, ok := listener.(*net.UnixListener); ok {
		os.Chmod(s.socketPath, 0600)
		fmt.Println("Unix socket permissions set to 0600")
	}

	// Accept connections in a goroutine
	go s.acceptConnections()

	// Handle outgoing messages in a goroutine
	go s.handleOutgoingMessages()

	return nil
}

func (s *IpcServer) acceptConnections() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}

		s.clientMu.Lock()
		s.clients[conn] = &sync.Mutex{}
		s.clientMu.Unlock()

		fmt.Printf("C++ daemon connected from: %s\n", conn.RemoteAddr())

		// Replay cached state messages to the new client
		go s.replayStateMessages(conn)

		go s.handleClient(conn)
	}
}

func (s *IpcServer) replayStateMessages(conn net.Conn) {
	s.latestMu.RLock()
	msgs := make([]*Message, 0, len(s.latestMessages))
	for _, msg := range s.latestMessages {
		msgs = append(msgs, msg)
	}
	s.latestMu.RUnlock()

	s.clientMu.Lock()
	mu, ok := s.clients[conn]
	s.clientMu.Unlock()
	if !ok {
		return
	}

	for _, msg := range msgs {
		mu.Lock()
		_ = WriteMessage(conn, msg)
		mu.Unlock()
	}
}

func (s *IpcServer) handleClient(conn net.Conn) {
	defer func() {
		s.clientMu.Lock()
		delete(s.clients, conn)
		s.clientMu.Unlock()
		conn.Close()
		fmt.Println("C++ daemon disconnected")
	}()

	for {
		msg, err := ReadMessage(conn)
		if err != nil {
			fmt.Printf("Error reading message: %v\n", err)
			return
		}

		if msg == nil {
			continue
		}

		fmt.Printf("Received from C++: %s\n", msg.Type)

		if s.OnMessage != nil {
			if err := s.OnMessage(msg); err != nil {
				fmt.Printf("Error handling message: %v\n", err)
			}
		}
	}
}

func (s *IpcServer) handleOutgoingMessages() {
	for {
		select {
		case <-s.stopChan:
			return
		case msg, ok := <-s.ToC:
			if !ok {
				return
			}
			s.clientMu.Lock()
			for conn, mu := range s.clients {
				go func(c net.Conn, m *sync.Mutex) {
					m.Lock()
					defer m.Unlock()
					_ = WriteMessage(c, msg)
				}(conn, mu)
			}
			s.clientMu.Unlock()
		}
	}
}

func (s *IpcServer) SendMessage(msg *Message) {
	defer func() {
		_ = recover()
	}()

	// Cache state messages for late-connecting clients
	if msg.Type == "peer_list_update" || msg.Type == "repo_list_response" {
		s.latestMu.Lock()
		s.latestMessages[msg.Type] = msg
		s.latestMu.Unlock()
	}

	select {
	case <-s.stopChan:
		// Server is shutting down, drop message
	case s.ToC <- msg:
	default:
		fmt.Println("Warning: IPC message queue full, dropping message")
	}
}

func (s *IpcServer) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})

	s.clientMu.Lock()
	for conn := range s.clients {
		conn.Close()
	}
	s.clientMu.Unlock()

	if s.listener != nil {
		s.listener.Close()
	}
	s.stopOnce.Do(func() {
		close(s.ToC)
	})
}

// readFull reads exactly len(buf) bytes from conn.
func readFull(conn net.Conn, buf []byte) error {
	for offset := 0; offset < len(buf); {
		n, err := conn.Read(buf[offset:])
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("unexpected EOF")
		}
		offset += n
	}
	return nil
}

// writeFull writes all bytes in buf to conn.
func writeFull(conn net.Conn, buf []byte) error {
	for offset := 0; offset < len(buf); {
		n, err := conn.Write(buf[offset:])
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("write returned 0 bytes")
		}
		offset += n
	}
	return nil
}

// ReadMessage reads a length-prefixed JSON message
func ReadMessage(conn net.Conn) (*Message, error) {
	// Read 4-byte length prefix (big-endian)
	lenBuf := make([]byte, 4)
	if err := readFull(conn, lenBuf); err != nil {
		return nil, err
	}

	len := binary.BigEndian.Uint32(lenBuf)
	if len == 0 || len > 1024*1024 { // 1MB limit
		return nil, fmt.Errorf("invalid message length: %d bytes", len)
	}

	// Read message body
	msgBuf := make([]byte, len)
	if err := readFull(conn, msgBuf); err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(msgBuf, &msg); err != nil {
		return nil, err
	}

	if msg.Type == "" {
		return nil, fmt.Errorf("invalid message: missing type")
	}

	return &msg, nil
}

// WriteMessage writes a length-prefixed JSON message
func WriteMessage(conn net.Conn, msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))

	if err := writeFull(conn, lenBuf); err != nil {
		return err
	}

	if err := writeFull(conn, data); err != nil {
		return err
	}
	return nil
}

func deriveFallbackPort(socketPath string) int {
	h := uint32(2166136261)
	for i := 0; i < len(socketPath); i++ {
		h = (h ^ uint32(socketPath[i])) * 16777619
	}
	return 10000 + int(h % 20000)
}
