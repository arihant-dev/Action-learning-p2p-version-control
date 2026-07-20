package sqlite

import (
	"database/sql"
	"fmt"
)

// SyncEvent represents a single synchronization event recorded in the
// sync_history table.
type SyncEvent struct {
	EventID      string
	RepositoryID string
	FilePath     string
	PeerID       string
	Timestamp    int64  // Unix milliseconds
	EventType    string // "send", "receive", "conflict_detected", "conflict_resolved"
	Status       string // "success", "failed", "pending"
}

// HistoryStore provides CRUD operations on the sync_history table.
type HistoryStore struct {
	db *sql.DB
}

// LogEvent inserts a new sync event into the history table.
func (s *HistoryStore) LogEvent(e *SyncEvent) error {
	_, err := s.db.Exec(`
		INSERT INTO sync_history
			(event_id, repository_id, file_path, peer_id, timestamp, event_type, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, e.EventID, e.RepositoryID, e.FilePath, e.PeerID,
		e.Timestamp, e.EventType, e.Status)
	if err != nil {
		return fmt.Errorf("log sync event: %w", err)
	}
	return nil
}

// GetByRepository returns all sync events for a given repository,
// ordered by timestamp descending (most recent first).
func (s *HistoryStore) GetByRepository(repoID string) ([]*SyncEvent, error) {
	rows, err := s.db.Query(`
		SELECT event_id, repository_id, file_path, peer_id,
		       timestamp, event_type, status
		FROM sync_history
		WHERE repository_id = ?
		ORDER BY timestamp DESC
	`, repoID)
	if err != nil {
		return nil, fmt.Errorf("get sync history: %w", err)
	}
	defer rows.Close()

	var events []*SyncEvent
	for rows.Next() {
		e := &SyncEvent{}
		if err := rows.Scan(
			&e.EventID, &e.RepositoryID, &e.FilePath, &e.PeerID,
			&e.Timestamp, &e.EventType, &e.Status,
		); err != nil {
			return nil, fmt.Errorf("scan sync event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetByFile returns all sync events for a specific file path within a
// repository, ordered by timestamp descending.
func (s *HistoryStore) GetByFile(repoID, filePath string) ([]*SyncEvent, error) {
	rows, err := s.db.Query(`
		SELECT event_id, repository_id, file_path, peer_id,
		       timestamp, event_type, status
		FROM sync_history
		WHERE repository_id = ? AND file_path = ?
		ORDER BY timestamp DESC
	`, repoID, filePath)
	if err != nil {
		return nil, fmt.Errorf("get file sync history: %w", err)
	}
	defer rows.Close()

	var events []*SyncEvent
	for rows.Next() {
		e := &SyncEvent{}
		if err := rows.Scan(
			&e.EventID, &e.RepositoryID, &e.FilePath, &e.PeerID,
			&e.Timestamp, &e.EventType, &e.Status,
		); err != nil {
			return nil, fmt.Errorf("scan sync event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// ClearByRepository removes all sync history for a given repository.
func (s *HistoryStore) ClearByRepository(repoID string) error {
	_, err := s.db.Exec(`
		DELETE FROM sync_history WHERE repository_id = ?
	`, repoID)
	if err != nil {
		return fmt.Errorf("clear sync history: %w", err)
	}
	return nil
}
