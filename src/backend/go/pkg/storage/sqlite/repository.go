package sqlite

import (
	"database/sql"
	"fmt"
	"time"
)

// Repository represents a tracked synchronization directory.
type Repository struct {
	ID        string
	LocalPath string
	Status    string // "active", "paused", "error"
	SyncMode  string // "auto", "manual"
	CreatedAt int64  // Unix milliseconds
	UpdatedAt int64  // Unix milliseconds
}

// RepositoryStore provides CRUD operations on the repositories table.
type RepositoryStore struct {
	db *sql.DB
}

// Save inserts or replaces a repository record. CreatedAt is preserved
// on conflict (upsert); UpdatedAt is always refreshed.
func (s *RepositoryStore) Save(r *Repository) error {
	now := time.Now().UnixMilli()
	if r.CreatedAt == 0 {
		r.CreatedAt = now
	}
	r.UpdatedAt = now

	_, err := s.db.Exec(`
		INSERT INTO repositories (id, local_path, status, sync_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			local_path = excluded.local_path,
			status     = excluded.status,
			sync_mode  = excluded.sync_mode,
			updated_at = excluded.updated_at
	`, r.ID, r.LocalPath, r.Status, r.SyncMode, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save repository: %w", err)
	}
	return nil
}

// Get retrieves a single repository by ID. Returns nil if not found.
func (s *RepositoryStore) Get(id string) (*Repository, error) {
	r := &Repository{}
	err := s.db.QueryRow(`
		SELECT id, local_path, status, sync_mode, created_at, updated_at
		FROM repositories WHERE id = ?
	`, id).Scan(&r.ID, &r.LocalPath, &r.Status, &r.SyncMode, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	return r, nil
}

// List returns all repository records.
func (s *RepositoryStore) List() ([]*Repository, error) {
	rows, err := s.db.Query(`
		SELECT id, local_path, status, sync_mode, created_at, updated_at
		FROM repositories ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	defer rows.Close()

	var repos []*Repository
	for rows.Next() {
		r := &Repository{}
		if err := rows.Scan(&r.ID, &r.LocalPath, &r.Status, &r.SyncMode, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan repository: %w", err)
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// Delete removes a repository by ID. Because of ON DELETE CASCADE,
// this also removes all associated file_metadata and sync_history rows.
func (s *RepositoryStore) Delete(id string) error {
	result, err := s.db.Exec(`DELETE FROM repositories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete repository: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delete repository: not found")
	}
	return nil
}
