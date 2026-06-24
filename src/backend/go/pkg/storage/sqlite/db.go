// Package sqlite provides a local SQLite-backed storage layer for
// persisting synchronization state, repository listings, file metadata,
// and sync history.
package sqlite

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// schemaStatements contains the DDL executed once when the database is
// first opened. Each statement is run inside a single transaction so
// that schema creation is atomic.
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS repositories (
		id         TEXT PRIMARY KEY,
		local_path TEXT NOT NULL,
		status     TEXT NOT NULL DEFAULT 'active',
		sync_mode  TEXT NOT NULL DEFAULT 'auto',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`,

	`CREATE TABLE IF NOT EXISTS file_metadata (
		repository_id       TEXT    NOT NULL,
		filepath            TEXT    NOT NULL,
		hash                TEXT    NOT NULL,
		size                INTEGER NOT NULL,
		version             INTEGER NOT NULL DEFAULT 1,
		local_last_modified INTEGER NOT NULL,
		is_deleted          INTEGER NOT NULL DEFAULT 0,
		updated_at          INTEGER NOT NULL,
		PRIMARY KEY (repository_id, filepath),
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
	)`,

	`CREATE TABLE IF NOT EXISTS sync_history (
		event_id      TEXT PRIMARY KEY,
		repository_id TEXT    NOT NULL,
		file_path     TEXT    NOT NULL,
		peer_id       TEXT    NOT NULL,
		timestamp     INTEGER NOT NULL,
		event_type    TEXT    NOT NULL,
		status        TEXT    NOT NULL DEFAULT 'pending',
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
	)`,
}

// DB wraps a *sql.DB connection to the local SQLite database and
// exposes store helpers for each domain (repositories, file metadata,
// sync history).
type DB struct {
	conn *sql.DB
}

// Open opens (or creates) the SQLite database at the given path and
// applies the schema. Pass ":memory:" for an in-memory database that
// is useful in tests.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// Verify the connection is alive.
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping sqlite db: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.applySchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return db, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the raw *sql.DB connection for advanced use-cases.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// applySchema runs all DDL statements inside a single transaction.
func (db *DB) applySchema() error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}

	for _, stmt := range schemaStatements {
		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			return fmt.Errorf("exec schema: %w", err)
		}
	}

	return tx.Commit()
}

// Repositories returns a RepositoryStore backed by this database.
func (db *DB) Repositories() *RepositoryStore {
	return &RepositoryStore{db: db.conn}
}

// Metadata returns a MetadataStore backed by this database.
func (db *DB) Metadata() *MetadataStore {
	return &MetadataStore{db: db.conn}
}

// History returns a HistoryStore backed by this database.
func (db *DB) History() *HistoryStore {
	return &HistoryStore{db: db.conn}
}
