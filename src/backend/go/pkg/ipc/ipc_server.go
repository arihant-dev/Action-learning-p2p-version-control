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
}

func NewIpcServer(socketPath string) *IpcServer {
	return &IpcServer{
		socketPath: socketPath,
		clients:    make(map[net.Conn]*sync.Mutex),
		ToC:        make(chan *Message, 100),
		stopChan:   make(chan struct{}),
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
		listener, err = net.Listen("tcp", "127.0.0.1:9999")
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

		go s.handleClient(conn)
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
	close(s.ToC)
}

// ReadMessage reads a length-prefixed JSON message
func ReadMessage(conn net.Conn) (*Message, error) {
	// Read 4-byte length prefix (big-endian)
	lenBuf := make([]byte, 4)
	n, err := conn.Read(lenBuf)
	if err != nil || n != 4 {
		return nil, err
	}

	len := binary.BigEndian.Uint32(lenBuf)
	if len > 1024*1024 { // 1MB limit
		return nil, fmt.Errorf("message too large: %d bytes", len)
	}

	// Read message body
	msgBuf := make([]byte, len)
	n, err = conn.Read(msgBuf)
	if err != nil || n != int(len) {
		return nil, err
	}

	var msg Message
	err = json.Unmarshal(msgBuf, &msg)
	if err != nil {
		return nil, err
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

	if _, err := conn.Write(lenBuf); err != nil {
		return err
	}

	if _, err := conn.Write(data); err != nil {
		return err
	}

	return nil
}
