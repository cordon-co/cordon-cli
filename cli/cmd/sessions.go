package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/reporoot"
	"github.com/cordon-co/cordon-cli/cli/internal/store"
	cordsync "github.com/cordon-co/cordon-cli/cli/internal/sync"
	"github.com/cordon-co/cordon-cli/cli/internal/transcript"
	"github.com/spf13/cobra"
)

var sessionsExtractBackground bool
const defaultExtractActivityWindow = time.Hour
const extractActivityWindowEnv = "CORDON_SESSIONS_EXTRACT_ACTIVITY_WINDOW"

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Manage agent session data",
}

var sessionsExtractCmd = &cobra.Command{
	Use:    "extract",
	Short:  "Extract token usage from agent transcripts",
	Hidden: true, // invoked by background spawn, not directly by users
	Args:   cobra.NoArgs,
	RunE:   runSessionsExtract,
}

func init() {
	sessionsExtractCmd.Flags().BoolVar(&sessionsExtractBackground, "background", false, "Run in background mode")
	sessionsCmd.AddCommand(sessionsExtractCmd)
}

func runSessionsExtract(cmd *cobra.Command, args []string) error {
	absRoot, _, err := reporoot.Find()
	if err != nil {
		return err
	}
	absRoot, err = filepath.Abs(absRoot)
	if err != nil {
		return err
	}

	if sessionsExtractBackground {
		return runExtractBackground(absRoot)
	}
	return runExtractForeground(absRoot)
}

// runExtractBackground acquires an exclusive lock, runs extraction, and writes
// .last_extract. Same locking pattern as runSyncBackground.
func runExtractBackground(absRoot string) error {
	perimeterID, err := store.ReadPerimeterID(absRoot)
	if err != nil {
		return err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	repoDir := filepath.Join(homeDir, ".cordon", "repos", perimeterID)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return err
	}

	// Acquire exclusive lock.
	lockPath := filepath.Join(repoDir, ".extract.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil // another extraction is running — exit silently
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	// Redirect output to log file.
	logPath := filepath.Join(repoDir, "extract.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()

	n, err := doExtract(absRoot, logFile)
	if err != nil {
		fmt.Fprintf(logFile, "extract error: %v\n", err)
		return err
	}

	fmt.Fprintf(logFile, "extract complete: %d sessions processed\n", n)
	return cordsync.TouchLastExtract(absRoot)
}

func runExtractForeground(absRoot string) error {
	n, err := doExtract(absRoot, os.Stderr)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Extracted %d sessions\n", n)
	return nil
}

// doExtract finds pending sessions and extracts transcript data for each.
func doExtract(absRoot string, logW *os.File) (int, error) {
	db, err := store.OpenDataDB(absRoot)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	if err := store.MigrateDataDB(db); err != nil {
		return 0, err
	}

	activityWindow := defaultExtractActivityWindow
	if raw := os.Getenv(extractActivityWindowEnv); raw != "" {
		if parsed, parseErr := time.ParseDuration(raw); parseErr != nil {
			fmt.Fprintf(logW, "extract: invalid %s=%q, using default %s: %v\n",
				extractActivityWindowEnv, raw, defaultExtractActivityWindow, parseErr)
		} else if parsed <= 0 {
			fmt.Fprintf(logW, "extract: non-positive %s=%q, using default %s\n",
				extractActivityWindowEnv, raw, defaultExtractActivityWindow)
		} else {
			activityWindow = parsed
		}
	}

	pending, err := store.PendingSessions(db, activityWindow)
	if err != nil {
		return 0, err
	}

	now := time.Now().UnixMicro()
	processed := 0
	for _, p := range pending {
		result, err := transcript.Extract(p.TranscriptPath, p.Agent)
		if err != nil {
			fmt.Fprintf(logW, "extract: session %s: %v\n", p.SessionID, err)
			continue
		}

		s := store.Session{
			SessionID:      p.SessionID,
			Agent:          p.Agent,
			TranscriptPath: p.TranscriptPath,
			FirstSeenAt:    p.FirstSeenAt,
			LastSeenAt:     p.LastSeenAt,
			UpdatedAt:      now,
		}

		if result != nil {
			s.Description = result.Description
			s.InputTokens = result.InputTokens
			s.OutputTokens = result.OutputTokens
			s.CacheReadTokens = result.CacheReadTokens
		}

		if err := store.UpsertSession(db, s); err != nil {
			fmt.Fprintf(logW, "extract: session %s: upsert: %v\n", p.SessionID, err)
			continue
		}
		processed++
	}

	return processed, nil
}
