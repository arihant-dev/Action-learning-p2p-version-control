package sync

import (
	"testing"
	"time"
)

func TestSyncQueuePushPop(t *testing.T) {
	q := NewSyncQueue()

	task1 := &SyncTask{RepoID: "repo1", FilePath: "file1.txt", Type: Upload, Size: 100, Timestamp: time.Now()}
	task2 := &SyncTask{RepoID: "repo1", FilePath: "file2.txt", Type: Upload, Size: 50, Timestamp: time.Now()}
	task3 := &SyncTask{RepoID: "repo2", FilePath: "file3.txt", Type: Download, Size: 200, Timestamp: time.Now()}

	q.Push(task1)
	q.Push(task2)
	q.Push(task3)

	if q.Size() != 3 {
		t.Errorf("expected size 3, got %d", q.Size())
	}

	popped := q.Pop()
	if popped == nil {
		t.Fatal("expected non-nil task")
	}

	if popped.RepoID == "repo1" {
		if popped.FilePath != "file2.txt" {
			t.Errorf("expected file2.txt (smaller size, higher priority), got %s", popped.FilePath)
		}
	}

	remaining := q.Size()
	if remaining != 2 {
		t.Errorf("expected size 2 after pop, got %d", remaining)
	}
}

func TestSyncQueueEmptyPop(t *testing.T) {
	q := NewSyncQueue()
	result := q.Pop()
	if result != nil {
		t.Error("expected nil from empty queue")
	}
}

func TestSyncQueueRoundRobin(t *testing.T) {
	q := NewSyncQueue()

	q.Push(&SyncTask{RepoID: "repo1", FilePath: "a.txt", Type: Upload, Size: 10, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "repo2", FilePath: "b.txt", Type: Upload, Size: 10, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "repo1", FilePath: "c.txt", Type: Upload, Size: 10, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "repo2", FilePath: "d.txt", Type: Upload, Size: 10, Timestamp: time.Now()})

	first := q.Pop()
	if first.RepoID != "repo1" {
		t.Errorf("expected repo1 first, got %s", first.RepoID)
	}

	second := q.Pop()
	if second.RepoID != "repo2" {
		t.Errorf("expected repo2 second (round-robin), got %s", second.RepoID)
	}

	third := q.Pop()
	if third.RepoID != "repo1" {
		t.Errorf("expected repo1 third (round-robin), got %s", third.RepoID)
	}

	fourth := q.Pop()
	if fourth.RepoID != "repo2" {
		t.Errorf("expected repo2 fourth (round-robin), got %s", fourth.RepoID)
	}
}

func TestSyncQueueDuplicateTask(t *testing.T) {
	q := NewSyncQueue()

	task := &SyncTask{RepoID: "repo1", FilePath: "file.txt", Type: Upload, Hash: "abc", Size: 100, Timestamp: time.Now()}
	q.Push(task)

	dup := &SyncTask{RepoID: "repo1", FilePath: "file.txt", Type: Upload, Hash: "abc", Size: 100, Timestamp: time.Now()}
	q.Push(dup)

	if q.Size() != 1 {
		t.Errorf("expected size 1 (duplicate rejected), got %d", q.Size())
	}
}

func TestSyncQueueRemoveRepository(t *testing.T) {
	q := NewSyncQueue()

	q.Push(&SyncTask{RepoID: "repo1", FilePath: "a.txt", Type: Upload, Size: 10, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "repo2", FilePath: "b.txt", Type: Upload, Size: 10, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "repo1", FilePath: "c.txt", Type: Upload, Size: 10, Timestamp: time.Now()})

	q.RemoveRepository("repo1")

	if q.Size() != 1 {
		t.Errorf("expected size 1 after removing repo1, got %d", q.Size())
	}

	task := q.Pop()
	if task == nil {
		t.Fatal("expected non-nil task")
	}
	if task.RepoID != "repo2" {
		t.Errorf("expected repo2, got %s", task.RepoID)
	}
}

func TestSyncQueuePriorityOrdering(t *testing.T) {
	q := NewSyncQueue()

	q.Push(&SyncTask{RepoID: "repo1", FilePath: "large.txt", Type: Upload, Size: 1000, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "repo1", FilePath: "small.txt", Type: Upload, Size: 10, Timestamp: time.Now()})
	q.Push(&SyncTask{RepoID: "repo1", FilePath: "medium.txt", Type: Upload, Size: 100, Timestamp: time.Now()})

	first := q.Pop()
	if first.FilePath != "small.txt" {
		t.Errorf("expected small.txt first (smallest = highest priority), got %s", first.FilePath)
	}

	second := q.Pop()
	if second.FilePath != "medium.txt" {
		t.Errorf("expected medium.txt second, got %s", second.FilePath)
	}

	third := q.Pop()
	if third.FilePath != "large.txt" {
		t.Errorf("expected large.txt third (largest = lowest priority), got %s", third.FilePath)
	}
}
