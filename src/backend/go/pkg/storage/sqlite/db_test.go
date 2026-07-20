package sqlite

import (
	"testing"
	"time"
)

// testDB returns a fresh in-memory database for each test.
func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ---------------------------------------------------------------------------
// Schema tests
// ---------------------------------------------------------------------------

func TestOpenCreatesSchema(t *testing.T) {
	db := testDB(t)

	// Verify all three tables exist by querying sqlite_master.
	tables := map[string]bool{"repositories": false, "file_metadata": false, "sync_history": false}
	rows, err := db.Conn().Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		if _, ok := tables[name]; ok {
			tables[name] = true
		}
	}

	for table, found := range tables {
		if !found {
			t.Errorf("expected table %q to exist", table)
		}
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	db := testDB(t)

	// Attempting to insert file_metadata with a non-existent repository_id
	// should fail because of the foreign key constraint.
	_, err := db.Conn().Exec(`
		INSERT INTO file_metadata
			(repository_id, filepath, hash, size, version, local_last_modified, is_deleted, updated_at)
		VALUES ('nonexistent', 'test.txt', 'abc', 100, 1, 0, 0, 0)
	`)
	if err == nil {
		t.Fatal("expected foreign key violation, got nil")
	}
}

// ---------------------------------------------------------------------------
// Repository tests
// ---------------------------------------------------------------------------

func TestRepositorySaveAndGet(t *testing.T) {
	db := testDB(t)
	store := db.Repositories()

	repo := &Repository{
		ID:        "repo-1",
		LocalPath: "/home/user/sync",
		Status:    "active",
		SyncMode:  "auto",
	}
	if err := store.Save(repo); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.Get("repo-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected repository, got nil")
	}
	if got.LocalPath != "/home/user/sync" {
		t.Errorf("local_path = %q, want %q", got.LocalPath, "/home/user/sync")
	}
	if got.Status != "active" {
		t.Errorf("status = %q, want %q", got.Status, "active")
	}
	if got.CreatedAt == 0 || got.UpdatedAt == 0 {
		t.Error("timestamps should be non-zero")
	}
}

func TestRepositoryGetNotFound(t *testing.T) {
	db := testDB(t)
	store := db.Repositories()

	got, err := store.Get("nonexistent")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestRepositoryUpsert(t *testing.T) {
	db := testDB(t)
	store := db.Repositories()

	repo := &Repository{
		ID:        "repo-1",
		LocalPath: "/original/path",
		Status:    "active",
		SyncMode:  "auto",
	}
	if err := store.Save(repo); err != nil {
		t.Fatalf("save: %v", err)
	}
	originalCreatedAt := repo.CreatedAt

	// Update path and status.
	time.Sleep(2 * time.Millisecond) // ensure UpdatedAt advances
	repo.LocalPath = "/updated/path"
	repo.Status = "paused"
	if err := store.Save(repo); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, _ := store.Get("repo-1")
	if got.LocalPath != "/updated/path" {
		t.Errorf("local_path = %q, want %q", got.LocalPath, "/updated/path")
	}
	if got.Status != "paused" {
		t.Errorf("status = %q, want %q", got.Status, "paused")
	}
	// CreatedAt should be preserved from the original insert because the
	// upsert's DO UPDATE clause does not touch created_at.
	if got.CreatedAt != originalCreatedAt {
		t.Errorf("created_at changed: got %d, want %d", got.CreatedAt, originalCreatedAt)
	}
}

func TestRepositoryList(t *testing.T) {
	db := testDB(t)
	store := db.Repositories()

	for _, id := range []string{"a", "b", "c"} {
		if err := store.Save(&Repository{ID: id, LocalPath: "/" + id, Status: "active", SyncMode: "auto"}); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}

	repos, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(repos) != 3 {
		t.Fatalf("len = %d, want 3", len(repos))
	}
}

func TestRepositoryDeleteCascade(t *testing.T) {
	db := testDB(t)
	repoStore := db.Repositories()
	metaStore := db.Metadata()
	histStore := db.History()

	// Create a repository with associated metadata and history.
	repo := &Repository{ID: "repo-1", LocalPath: "/test", Status: "active", SyncMode: "auto"}
	if err := repoStore.Save(repo); err != nil {
		t.Fatalf("save repo: %v", err)
	}
	if err := metaStore.Save(&FileMetadata{
		RepositoryID: "repo-1", Filepath: "a.txt", Hash: "abc",
		Size: 10, Version: 1, LocalLastModified: 100,
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	if err := histStore.LogEvent(&SyncEvent{
		EventID: "ev-1", RepositoryID: "repo-1", FilePath: "a.txt",
		PeerID: "peer-1", Timestamp: 200, EventType: "send", Status: "success",
	}); err != nil {
		t.Fatalf("log event: %v", err)
	}

	// Delete the repository — should cascade to metadata and history.
	if err := repoStore.Delete("repo-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Verify cascade: metadata gone.
	files, _ := metaStore.ListByRepository("repo-1", true)
	if len(files) != 0 {
		t.Errorf("expected 0 metadata rows after cascade, got %d", len(files))
	}

	// Verify cascade: history gone.
	events, _ := histStore.GetByRepository("repo-1")
	if len(events) != 0 {
		t.Errorf("expected 0 history rows after cascade, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// File metadata tests
// ---------------------------------------------------------------------------

func TestFileMetadataSaveAndGet(t *testing.T) {
	db := testDB(t)
	repoStore := db.Repositories()
	metaStore := db.Metadata()

	// Must create parent repository first (foreign key).
	repoStore.Save(&Repository{ID: "repo-1", LocalPath: "/test", Status: "active", SyncMode: "auto"})

	meta := &FileMetadata{
		RepositoryID:      "repo-1",
		Filepath:          "docs/readme.md",
		Hash:              "sha256:abcdef",
		Size:              1024,
		Version:           1,
		LocalLastModified: time.Now().UnixMilli(),
	}
	if err := metaStore.Save(meta); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := metaStore.Get("repo-1", "docs/readme.md")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected metadata, got nil")
	}
	if got.Hash != "sha256:abcdef" {
		t.Errorf("hash = %q, want %q", got.Hash, "sha256:abcdef")
	}
	if got.Size != 1024 {
		t.Errorf("size = %d, want 1024", got.Size)
	}
	if got.Version != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
}

func TestFileMetadataSoftDelete(t *testing.T) {
	db := testDB(t)
	repoStore := db.Repositories()
	metaStore := db.Metadata()

	repoStore.Save(&Repository{ID: "repo-1", LocalPath: "/test", Status: "active", SyncMode: "auto"})
	metaStore.Save(&FileMetadata{
		RepositoryID: "repo-1", Filepath: "a.txt", Hash: "abc",
		Size: 10, Version: 1, LocalLastModified: 100,
	})

	// Soft-delete the file.
	if err := metaStore.SoftDelete("repo-1", "a.txt"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	// Should still be retrievable via Get.
	got, _ := metaStore.Get("repo-1", "a.txt")
	if got == nil {
		t.Fatal("expected tombstoned metadata, got nil")
	}
	if !got.IsDeleted {
		t.Error("expected is_deleted = true")
	}

	// ListByRepository with includeDeleted=false should exclude it.
	files, _ := metaStore.ListByRepository("repo-1", false)
	if len(files) != 0 {
		t.Errorf("expected 0 active files, got %d", len(files))
	}

	// ListByRepository with includeDeleted=true should include it.
	files, _ = metaStore.ListByRepository("repo-1", true)
	if len(files) != 1 {
		t.Errorf("expected 1 file (tombstoned), got %d", len(files))
	}
}

func TestFileMetadataVersionUpdate(t *testing.T) {
	db := testDB(t)
	repoStore := db.Repositories()
	metaStore := db.Metadata()

	repoStore.Save(&Repository{ID: "repo-1", LocalPath: "/test", Status: "active", SyncMode: "auto"})

	// Version 1.
	metaStore.Save(&FileMetadata{
		RepositoryID: "repo-1", Filepath: "a.txt", Hash: "hash-v1",
		Size: 10, Version: 1, LocalLastModified: 100,
	})

	// Version 2 — same key, updated hash and version.
	metaStore.Save(&FileMetadata{
		RepositoryID: "repo-1", Filepath: "a.txt", Hash: "hash-v2",
		Size: 20, Version: 2, LocalLastModified: 200,
	})

	got, _ := metaStore.Get("repo-1", "a.txt")
	if got.Version != 2 {
		t.Errorf("version = %d, want 2", got.Version)
	}
	if got.Hash != "hash-v2" {
		t.Errorf("hash = %q, want %q", got.Hash, "hash-v2")
	}
}

// ---------------------------------------------------------------------------
// Sync history tests
// ---------------------------------------------------------------------------

func TestSyncHistoryLogAndQuery(t *testing.T) {
	db := testDB(t)
	repoStore := db.Repositories()
	histStore := db.History()

	repoStore.Save(&Repository{ID: "repo-1", LocalPath: "/test", Status: "active", SyncMode: "auto"})

	events := []*SyncEvent{
		{EventID: "ev-1", RepositoryID: "repo-1", FilePath: "a.txt", PeerID: "peer-A", Timestamp: 100, EventType: "send", Status: "success"},
		{EventID: "ev-2", RepositoryID: "repo-1", FilePath: "b.txt", PeerID: "peer-B", Timestamp: 200, EventType: "receive", Status: "success"},
		{EventID: "ev-3", RepositoryID: "repo-1", FilePath: "a.txt", PeerID: "peer-A", Timestamp: 300, EventType: "conflict_detected", Status: "pending"},
	}
	for _, e := range events {
		if err := histStore.LogEvent(e); err != nil {
			t.Fatalf("log event %s: %v", e.EventID, err)
		}
	}

	// Query all events for repository — should be ordered by timestamp DESC.
	got, err := histStore.GetByRepository("repo-1")
	if err != nil {
		t.Fatalf("get by repo: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].EventID != "ev-3" {
		t.Errorf("first event = %q, want ev-3 (most recent)", got[0].EventID)
	}

	// Query by file — should return only events for a.txt.
	fileEvents, err := histStore.GetByFile("repo-1", "a.txt")
	if err != nil {
		t.Fatalf("get by file: %v", err)
	}
	if len(fileEvents) != 2 {
		t.Fatalf("len = %d, want 2", len(fileEvents))
	}
}

func TestSyncHistoryClear(t *testing.T) {
	db := testDB(t)
	repoStore := db.Repositories()
	histStore := db.History()

	repoStore.Save(&Repository{ID: "repo-1", LocalPath: "/test", Status: "active", SyncMode: "auto"})
	histStore.LogEvent(&SyncEvent{
		EventID: "ev-1", RepositoryID: "repo-1", FilePath: "a.txt",
		PeerID: "peer-A", Timestamp: 100, EventType: "send", Status: "success",
	})

	if err := histStore.ClearByRepository("repo-1"); err != nil {
		t.Fatalf("clear: %v", err)
	}

	events, _ := histStore.GetByRepository("repo-1")
	if len(events) != 0 {
		t.Errorf("expected 0 events after clear, got %d", len(events))
	}
}
