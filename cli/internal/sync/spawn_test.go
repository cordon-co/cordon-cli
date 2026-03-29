package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncDueLogic(t *testing.T) {
	// Create a temp directory to simulate the .last_sync check.
	// We can't easily test SyncDue directly since it reads perimeter_id from
	// policy.db, but we can test the time-based logic via lastSyncPath simulation.

	tmpDir := t.TempDir()
	syncFile := filepath.Join(tmpDir, ".last_sync")

	// Helper that checks if sync is due based on the file.
	isDue := func() bool {
		info, err := os.Stat(syncFile)
		if err != nil {
			return true // missing = due
		}
		return time.Since(info.ModTime()) > syncInterval
	}

	// No file = sync is due.
	if !isDue() {
		t.Error("expected sync to be due when file is missing")
	}

	// Write the file now = sync is NOT due.
	if err := os.WriteFile(syncFile, []byte("now"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isDue() {
		t.Error("expected sync NOT to be due immediately after writing .last_sync")
	}

	// Backdate the file to 2 seconds ago = sync IS due.
	old := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(syncFile, old, old); err != nil {
		t.Fatal(err)
	}
	if !isDue() {
		t.Error("expected sync to be due after 2 seconds")
	}

	// Set file to now = sync is NOT due (within 1s interval).
	recent := time.Now()
	if err := os.Chtimes(syncFile, recent, recent); err != nil {
		t.Fatal(err)
	}
	if isDue() {
		t.Error("expected sync NOT to be due within 1s interval")
	}
}
