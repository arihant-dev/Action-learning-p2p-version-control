package sync

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

func TestQueueRoundRobinFairness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		q := NewSyncQueue()

		repoIDs := []string{"RepoA", "RepoB", "RepoC"}
		taskCount := rapid.IntRange(1, 20).Draw(t, "tasksPerRepo")

		for _, repoID := range repoIDs {
			for i := 0; i < taskCount; i++ {
				q.Push(&SyncTask{
					RepoID:    repoID,
					FilePath:  "file_" + repoID + "_" + string(rune('a'+i)),
					Type:      Download,
					Size:      int64(rapid.Int64Range(1, 1000).Draw(t, "size")),
					Timestamp: time.Now(),
				})
			}
		}

		popped := map[string]int{}
		for {
			task := q.Pop()
			if task == nil {
				break
			}
			popped[task.RepoID]++
		}

		for _, repoID := range repoIDs {
			if popped[repoID] != taskCount {
				t.Fatalf("repo %s: expected %d pops, got %d", repoID, taskCount, popped[repoID])
			}
		}
	})
}

func TestQueueFIFOWithinRepo(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		q := NewSyncQueue()

		taskCount := rapid.IntRange(2, 10).Draw(t, "count")
		timestamps := make([]time.Time, taskCount)
		now := time.Now()

		for i := 0; i < taskCount; i++ {
			ts := now.Add(time.Duration(i) * time.Second)
			timestamps[i] = ts
			q.Push(&SyncTask{
				RepoID:    "RepoA",
				FilePath:  "file_" + string(rune('a'+i)) + ".txt",
				Type:      Download,
				Size:      100,
				Timestamp: ts,
			})
		}

		type namedTask struct {
			path string
			ts   time.Time
		}
		var popped []namedTask
		for {
			task := q.Pop()
			if task == nil || task.RepoID != "RepoA" {
				break
			}
			popped = append(popped, namedTask{path: task.FilePath, ts: task.Timestamp})
		}

		if len(popped) != taskCount {
			t.Fatalf("expected %d tasks, popped %d", taskCount, len(popped))
		}

		for i := 1; i < len(popped); i++ {
			if popped[i].ts.Before(popped[i-1].ts) {
				t.Fatalf("FIFO violation: task %s (ts=%v) popped before %s (ts=%v)",
					popped[i].path, popped[i].ts, popped[i-1].path, popped[i-1].ts)
			}
		}
	})
}

func TestQueueDeduplication(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		q := NewSyncQueue()

		q.Push(&SyncTask{RepoID: "R1", FilePath: "f.txt", Type: Download, Hash: "h1", Size: 10, Timestamp: time.Now()})
		q.Push(&SyncTask{RepoID: "R1", FilePath: "f.txt", Type: Download, Hash: "h1", Size: 10, Timestamp: time.Now()})

		if q.Size() != 1 {
			t.Fatalf("expected dedup to 1, got %d", q.Size())
		}

		q.Push(&SyncTask{RepoID: "R1", FilePath: "f.txt", Type: Download, Hash: "h2", Size: 20, Timestamp: time.Now()})

		if q.Size() != 2 {
			t.Fatalf("expected size 2 after different hash, got %d", q.Size())
		}
	})
}
