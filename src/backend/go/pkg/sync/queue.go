package sync

import (
	"sort"
	"sync"
	"time"
)

// TaskType describes the direction of the sync transfer.
type TaskType string

const (
	Upload   TaskType = "upload"
	Download TaskType = "download"
)

// SyncTask represents a single file synchronization unit.
type SyncTask struct {
	RepoID    string
	FilePath  string
	Type      TaskType
	Hash      string
	Size      int64
	Timestamp time.Time
	PeerID    string // Target peer to sync with
	Mode      uint32
}

// Priority computes the priority of a task (lower values mean higher priority).
// Rules:
// 1. Smaller file sizes are processed first to ensure metadata/small files sync quickly.
// 2. Tie-break by older timestamps first.
func (t *SyncTask) Priority() int64 {
	// Size is the main priority driver.
	return t.Size
}

// SyncQueue is a thread-safe queue that schedules tasks fairly across multiple
// repositories using round-robin scheduling, and prioritises small files within each repo.
type SyncQueue struct {
	mu       sync.Mutex
	queues   map[string][]*SyncTask // repoID -> slice of tasks
	repos    []string               // order of repositories for round-robin
	nextRepo int                    // pointer to the next repository index
}

// NewSyncQueue creates a new SyncQueue.
func NewSyncQueue() *SyncQueue {
	return &SyncQueue{
		queues: make(map[string][]*SyncTask),
		repos:  make([]string, 0),
	}
}

// Push adds a task to the queue, maintaining priority order (smaller files first).
func (q *SyncQueue) Push(task *SyncTask) {
	q.mu.Lock()
	defer q.mu.Unlock()

	repoID := task.RepoID
	tasks, exists := q.queues[repoID]
	if !exists {
		q.queues[repoID] = []*SyncTask{task}
		q.repos = append(q.repos, repoID)
	} else {
		// Avoid duplicate sync tasks for the same file in the same direction.
		for _, t := range tasks {
			if t.FilePath == task.FilePath && t.Type == task.Type && t.Hash == task.Hash {
				return
			}
		}
		q.queues[repoID] = append(tasks, task)
	}

	// Sort tasks: smaller size first, tie-break by older timestamp first.
	sort.Slice(q.queues[repoID], func(i, j int) bool {
		pi := q.queues[repoID][i]
		pj := q.queues[repoID][j]
		if pi.Priority() == pj.Priority() {
			return pi.Timestamp.Before(pj.Timestamp)
		}
		return pi.Priority() < pj.Priority()
	})
}

// Pop retrieves the next task using round-robin repository scheduling.
// Returns nil if the queue is empty.
func (q *SyncQueue) Pop() *SyncTask {
	q.mu.Lock()
	defer q.mu.Unlock()

	numRepos := len(q.repos)
	if numRepos == 0 {
		return nil
	}

	// Cycle through repos starting at nextRepo to find a task.
	for i := 0; i < numRepos; i++ {
		idx := (q.nextRepo + i) % numRepos
		repoID := q.repos[idx]
		tasks := q.queues[repoID]

		if len(tasks) > 0 {
			task := tasks[0]
			q.queues[repoID] = tasks[1:]

			// Update nextRepo pointer for the next pop
			q.nextRepo = (idx + 1) % numRepos
			return task
		}
	}

	return nil
}

// Requeue inserts a task at the front of its repo's queue, bypassing dedup.
// Used to retry tasks that couldn't be processed immediately (e.g. no semaphore).
func (q *SyncQueue) Requeue(task *SyncTask) {
	q.mu.Lock()
	defer q.mu.Unlock()
	repoID := task.RepoID
	tasks := q.queues[repoID]
	q.queues[repoID] = append([]*SyncTask{task}, tasks...)
}

// HasPending checks if a Download task for the given path and hash already exists in the queue.
func (q *SyncQueue) HasPending(repoID, path, hash string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	tasks, exists := q.queues[repoID]
	if !exists {
		return false
	}
	for _, t := range tasks {
		if t.FilePath == path && t.Type == Download && t.Hash == hash {
			return true
		}
	}
	return false
}

// RemoveRepository removes all pending tasks for a given repository.
func (q *SyncQueue) RemoveRepository(repoID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.queues, repoID)

	// Remove from repos list
	for i, r := range q.repos {
		if r == repoID {
			q.repos = append(q.repos[:i], q.repos[i+1:]...)
			break
		}
	}

	// Reset nextRepo index if it's out of bounds
	if len(q.repos) > 0 && q.nextRepo >= len(q.repos) {
		q.nextRepo = 0
	}
}

// Size returns the total number of tasks in the queue.
func (q *SyncQueue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	total := 0
	for _, tasks := range q.queues {
		total += len(tasks)
	}
	return total
}
