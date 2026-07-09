package sync

import (
	"math/rand"
	"testing"
	"time"
)

func BenchmarkSyncQueuePush(b *testing.B) {
	q := NewSyncQueue()
	repoIDs := []string{"repo-alpha", "repo-beta", "repo-gamma"}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		task := &SyncTask{
			RepoID:    repoIDs[rand.Intn(len(repoIDs))],
			FilePath:  "file-" + string(rune('a'+i%26)) + ".txt",
			Type:      Upload,
			Hash:      "abc123def456",
			Size:      int64(rand.Intn(10 * 1024 * 1024)),
			Timestamp: time.Now(),
		}
		q.Push(task)
	}
}

func BenchmarkSyncQueuePop(b *testing.B) {
	q := NewSyncQueue()
	repoIDs := []string{"repo-alpha", "repo-beta", "repo-gamma"}

	for i := 0; i < 10000; i++ {
		q.Push(&SyncTask{
			RepoID:    repoIDs[rand.Intn(len(repoIDs))],
			FilePath:  "file-" + string(rune('a'+i%26)) + ".txt",
			Type:      Download,
			Hash:      "hash-" + string(rune('0'+i%10)),
			Size:      int64(rand.Intn(10 * 1024 * 1024)),
			Timestamp: time.Now(),
		})
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		task := q.Pop()
		if task == nil {
			b.Fatal("queue empty before benchmark completed")
		}
	}
}

func BenchmarkSyncQueueRoundRobin(b *testing.B) {
	q := NewSyncQueue()
	repoIDs := []string{"repo-alpha", "repo-beta", "repo-gamma", "repo-delta"}

	for _, repoID := range repoIDs {
		for i := 0; i < 100; i++ {
			q.Push(&SyncTask{
				RepoID:    repoID,
				FilePath:  "file-" + string(rune('a'+i%26)) + ".txt",
				Type:      Upload,
				Size:      int64(rand.Intn(1024 * 1024)),
				Timestamp: time.Now(),
			})
		}
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		task := q.Pop()
		if task == nil {
			b.Fatal("queue empty")
		}
	}
}

func BenchmarkSyncQueueConcurrent(b *testing.B) {
	q := NewSyncQueue()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			task := &SyncTask{
				RepoID:    "repo-concurrent",
				FilePath:  "concurrent-file.txt",
				Type:      Upload,
				Hash:      "concurrent-hash",
				Size:      int64(rand.Intn(1024)),
				Timestamp: time.Now(),
			}
			q.Push(task)
			q.Pop()
		}
	})
}
