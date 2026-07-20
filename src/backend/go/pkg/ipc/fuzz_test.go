package ipc

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net"
	"testing"
	"time"
)

type mockConn struct {
	*bytes.Buffer
}

func (m *mockConn) Read(b []byte) (int, error)         { return m.Buffer.Read(b) }
func (m *mockConn) Write(b []byte) (int, error)        { return m.Buffer.Write(b) }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0} }
func (m *mockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0} }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func FuzzReadMessage(f *testing.F) {
	validMsg := &Message{
		Version:   "1.0",
		Type:      "ping",
		ID:        "msg_1",
		Timestamp: 1704067200000,
		Source:    "go",
		Payload:   json.RawMessage(`{}`),
	}
	data, _ := json.Marshal(validMsg)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	f.Add(append(lenBuf, data...))

	f.Add([]byte{0, 0, 0, 5, 104, 101, 108, 108, 111})
	f.Add([]byte{0, 0, 0, 0})
	f.Add([]byte{0, 0, 0, 3, 49, 50, 51})

	f.Fuzz(func(t *testing.T, data []byte) {
		conn := &mockConn{bytes.NewBuffer(data)}
		msg, err := ReadMessage(conn)
		if err == nil && msg != nil {
			if msg.Type == "" {
				t.Errorf("message with no type")
			}
		}
	})
}
