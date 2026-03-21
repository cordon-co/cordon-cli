package store

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestPolicyDB opens an in-memory policy database and runs all migrations.
func newTestPolicyDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory policy db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := MigratePolicyDB(db); err != nil {
		t.Fatalf("migrate policy db: %v", err)
	}
	return db
}

// newTestDataDB opens an in-memory data database and runs all migrations.
func newTestDataDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory data db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := MigrateDataDB(db); err != nil {
		t.Fatalf("migrate data db: %v", err)
	}
	return db
}
