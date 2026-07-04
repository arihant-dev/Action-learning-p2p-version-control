package sqlite

import (
	"database/sql"
	"fmt"
	"time"
)

// FileMetadata represents the tracked state of a single file inside a
// synced repository.
type FileMetadata struct {
	RepositoryID      string
	Filepath          string
	Hash              string
	Size              int64
	Version           int64 // Lamport clock / version counter
	LocalLastModified int64 // Unix milliseconds
	IsDeleted         bool  // Tombstone flag
	UpdatedAt         int64 // Unix milliseconds
	Mode              uint32
}

// MetadataStore provides CRUD operations on the file_metadata table.
type MetadataStore struct {
	db *sql.DB
}

// Save inserts or updates a file metadata record. Version and
// UpdatedAt are always refreshed on conflict.
func (s *MetadataStore) Save(m *FileMetadata) error {
	now := time.Now().UnixMilli()
	m.UpdatedAt = now

	deleted := 0
	if m.IsDeleted {
		deleted = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO file_metadata
			(repository_id, filepath, hash, size, version, local_last_modified, is_deleted, updated_at, mode)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id, filepath) DO UPDATE SET
			hash                = excluded.hash,
			size                = excluded.size,
			version             = excluded.version,
			local_last_modified = excluded.local_last_modified,
			is_deleted          = excluded.is_deleted,
			updated_at          = excluded.updated_at,
			mode                = excluded.mode
	`, m.RepositoryID, m.Filepath, m.Hash, m.Size, m.Version,
		m.LocalLastModified, deleted, m.UpdatedAt, m.Mode)
	if err != nil {
		return fmt.Errorf("save file metadata: %w", err)
	}
	return nil
}

// Get retrieves file metadata by repository ID and filepath.
// Returns nil if not found.
func (s *MetadataStore) Get(repoID, filepath string) (*FileMetadata, error) {
	m := &FileMetadata{}
	var deleted int
	err := s.db.QueryRow(`
		SELECT repository_id, filepath, hash, size, version,
		       local_last_modified, is_deleted, updated_at, mode
		FROM file_metadata
		WHERE repository_id = ? AND filepath = ?
	`, repoID, filepath).Scan(
		&m.RepositoryID, &m.Filepath, &m.Hash, &m.Size, &m.Version,
		&m.LocalLastModified, &deleted, &m.UpdatedAt, &m.Mode,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get file metadata: %w", err)
	}
	m.IsDeleted = deleted != 0
	return m, nil
}

// GetByPathAndHash retrieves file metadata across all repositories by filepath and hash.
// Returns nil if not found or if the file is marked as deleted.
func (s *MetadataStore) GetByPathAndHash(filepath, hash string) (*FileMetadata, error) {
	m := &FileMetadata{}
	var deleted int
	err := s.db.QueryRow(`
		SELECT repository_id, filepath, hash, size, version,
		       local_last_modified, is_deleted, updated_at, mode
		FROM file_metadata
		WHERE filepath = ? AND hash = ? AND is_deleted = 0
	`, filepath, hash).Scan(
		&m.RepositoryID, &m.Filepath, &m.Hash, &m.Size, &m.Version,
		&m.LocalLastModified, &deleted, &m.UpdatedAt, &m.Mode,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get file metadata by path and hash: %w", err)
	}
	m.IsDeleted = deleted != 0
	return m, nil
}

// ListByRepository returns all file metadata records for a given
// repository, optionally including soft-deleted (tombstoned) entries.
func (s *MetadataStore) ListByRepository(repoID string, includeDeleted bool) ([]*FileMetadata, error) {
	query := `
		SELECT repository_id, filepath, hash, size, version,
		       local_last_modified, is_deleted, updated_at, mode
		FROM file_metadata
		WHERE repository_id = ?
	`
	if !includeDeleted {
		query += ` AND is_deleted = 0`
	}
	query += ` ORDER BY filepath ASC`

	rows, err := s.db.Query(query, repoID)
	if err != nil {
		return nil, fmt.Errorf("list file metadata: %w", err)
	}
	defer rows.Close()

	var files []*FileMetadata
	for rows.Next() {
		m := &FileMetadata{}
		var deleted int
		if err := rows.Scan(
			&m.RepositoryID, &m.Filepath, &m.Hash, &m.Size, &m.Version,
			&m.LocalLastModified, &deleted, &m.UpdatedAt, &m.Mode,
		); err != nil {
			return nil, fmt.Errorf("scan file metadata: %w", err)
		}
		m.IsDeleted = deleted != 0
		files = append(files, m)
	}
	return files, rows.Err()
}

// SoftDelete marks a file as deleted (tombstone) without physically
// removing the row, so that the deletion can be propagated to peers.
func (s *MetadataStore) SoftDelete(repoID, filepath string) error {
	now := time.Now().UnixMilli()
	result, err := s.db.Exec(`
		UPDATE file_metadata
		SET is_deleted = 1, updated_at = ?
		WHERE repository_id = ? AND filepath = ?
	`, now, repoID, filepath)
	if err != nil {
		return fmt.Errorf("soft delete file metadata: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("soft delete file metadata: not found")
	}
	return nil
}

// HardDelete physically removes a file metadata record.
func (s *MetadataStore) HardDelete(repoID, filepath string) error {
	result, err := s.db.Exec(`
		DELETE FROM file_metadata
		WHERE repository_id = ? AND filepath = ?
	`, repoID, filepath)
	if err != nil {
		return fmt.Errorf("hard delete file metadata: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("hard delete file metadata: not found")
	}
	return nil
}
