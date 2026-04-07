package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateDataDB_CreatesBackupBeforeStamping(t *testing.T) {
	tmp := t.TempDir()
	dataPath := filepath.Join(tmp, "data.db")

	db, err := sql.Open("sqlite", dataPath)
	if err != nil {
		t.Fatalf("open data db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS legacy_table (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("seed legacy table: %v", err)
	}

	if err := MigrateDataDB(db); err != nil {
		t.Fatalf("migrate data db: %v", err)
	}

	backupPath := filepath.Join(tmp, "data.db.dev.install.backup")
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected backup at %s: %v", backupPath, err)
	}
}

func TestMigratePolicyDB_BackupStoredAlongsideDataDB(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	repoRoot := filepath.Join(tmp, "repo")
	policyDir := filepath.Join(repoRoot, ".cordon")
	if err := os.MkdirAll(policyDir, 0o755); err != nil {
		t.Fatalf("create policy dir: %v", err)
	}
	policyPath := filepath.Join(policyDir, "policy.db")

	db, err := sql.Open("sqlite", policyPath)
	if err != nil {
		t.Fatalf("open policy db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS perimeter_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create perimeter_meta: %v", err)
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO perimeter_meta (key, value) VALUES ('perimeter_id', 'perim123')`); err != nil {
		t.Fatalf("insert perimeter_id: %v", err)
	}

	if err := MigratePolicyDB(db); err != nil {
		t.Fatalf("migrate policy db: %v", err)
	}

	backupPath := filepath.Join(tmp, ".cordon", "repos", "perim123", "policy.db.dev.install.backup")
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected policy backup at %s: %v", backupPath, err)
	}
}

func TestMigrateDataDB_RejectsNewerSchemaVersion(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create schema_meta: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_meta (key, value) VALUES ('schema_version', '999')`); err != nil {
		t.Fatalf("seed schema version: %v", err)
	}

	err = MigrateDataDB(db)
	if err == nil {
		t.Fatal("expected migration to fail for newer schema")
	}
	if !strings.Contains(err.Error(), "newer than this cordon build supports") {
		t.Fatalf("unexpected error: %v", err)
	}
}
