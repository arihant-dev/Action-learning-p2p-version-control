package network

import (
	"encoding/json"
	"net"
	"sync"
	"testing"

	"p2p/pkg/ipc"
)

func BenchmarkBroadcastParallel(b *testing.B) {
	cm := NewConnectionManager("bench-peer")
	for i := 0; i < 50; i++ {
		id := "peer-" + string(rune('A'+i%26)) + string(rune('0'+i/10))
		cm.connections[id] = nil
		cm.writeMus[id] = &sync.Mutex{}
	}
	b.ResetTimer()

	msg := &ipc.Message{
		Version: "1.0",
		Type:    "bench",
		Source:  "bench",
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cm.Broadcast(msg)
		}
	})
}

func BenchmarkHandshakeSerialization(b *testing.B) {
	payload := HandshakePayload{
		PeerID: "bench-peer-001",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, _ := json.Marshal(payload)
		var decoded HandshakePayload
		_ = json.Unmarshal(data, &decoded)
	}
	b.StopTimer()
}

func BenchmarkSendToPeer(b *testing.B) {
	cm := NewConnectionManager("bench-peer")
	server, client := net.Pipe()
	cm.connections["target"] = client
	cm.writeMus["target"] = &sync.Mutex{}
	defer server.Close()
	defer cm.CloseConnection("target")

	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := server.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	msg := &ipc.Message{
		Version: "1.0",
		Type:    "bench",
		Source:  "bench",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cm.SendToPeer("target", msg)
	}
}
