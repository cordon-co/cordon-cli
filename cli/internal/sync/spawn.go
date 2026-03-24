package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/api"
	"github.com/cordon-co/cordon-cli/cli/internal/store"
)

const syncInterval = 60 * time.Second

// SpawnBackgroundSync spawns `cordon sync --background` as a fully detached
// process. The child process inherits no stdio and runs in a new session
// so it survives the parent (hook) exiting.
func SpawnBackgroundSync(absRepoRoot string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}

	cmd := exec.Command(exe, "sync", "--background")
	cmd.Dir = absRepoRoot
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	_ = cmd.Start()
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
}

// SyncDue returns true if no sync has occurred within the last 60 seconds.
// Returns true if the .last_sync file is missing or older than the interval.
func SyncDue(absRepoRoot string) bool {
	syncFile, err := lastSyncPath(absRepoRoot)
	if err != nil {
		return true
	}

	info, err := os.Stat(syncFile)
	if err != nil {
		return true // missing file = sync is due
	}

	return time.Since(info.ModTime()) > syncInterval
}

// SyncDueForNotification always returns true if the user is authenticated,
// bypassing the timer. Used when a hook matches a rule with the notify flag.
func SyncDueForNotification(absRepoRoot string) bool {
	return api.IsLoggedIn()
}

// TouchLastSync writes the current time to the .last_sync file.
func TouchLastSync(absRepoRoot string) error {
	syncFile, err := lastSyncPath(absRepoRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(syncFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(syncFile, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
}

// lastSyncPath returns the path to ~/.cordon/repos/<perimeter_id>/.last_sync.
func lastSyncPath(absRepoRoot string) (string, error) {
	id, err := store.ReadPerimeterID(absRepoRoot)
	if err != nil {
		return "", err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".cordon", "repos", id, ".last_sync"), nil
}
