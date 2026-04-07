package cmd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/cordon-co/cordon-cli/cli/internal/reporoot"
	"github.com/cordon-co/cordon-cli/cli/internal/store"
	"github.com/cordon-co/cordon-cli/cli/internal/tui"
	"github.com/spf13/cobra"
)

var logFile string
var logAllow bool
var logDeny bool
var logGranted bool
var logPass bool
var logSince string
var logUntil string
var logDate string
var logAgent string
var logFollow bool
var logInteractive bool
var logExport string
var logLimit int

// logCmd — named logCmd to avoid shadowing the standard library "log" package.
var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Display the audit log",
	Args:  cobra.NoArgs,
	RunE:  runLog,
}

func init() {
	logCmd.Flags().StringVar(&logFile, "file", "", "Filter by file path (substring match)")
	logCmd.Flags().BoolVar(&logAllow, "allow", false, "Show allowed hook events")
	logCmd.Flags().BoolVar(&logDeny, "deny", false, "Show denied hook events")
	logCmd.Flags().BoolVar(&logGranted, "granted", false, "Show hook events authorized by a pass")
	logCmd.Flags().BoolVar(&logPass, "pass", false, "Show pass lifecycle events (issue, revoke, expire)")
	logCmd.Flags().StringVar(&logSince, "since", "", "Show entries newer than duration (e.g. 24h, 7d, 90m)")
	logCmd.Flags().StringVar(&logUntil, "until", "", "Show entries older than time (RFC3339) or date (YYYY-MM-DD)")
	logCmd.Flags().StringVar(&logDate, "date", "", "Show entries for a specific date (e.g. 2026-03-22)")
	logCmd.Flags().StringVar(&logAgent, "agent", "", "Filter by agent platform (e.g. claude-code, cursor)")
	logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "Stream new entries in real-time")
	logCmd.Flags().BoolVarP(&logInteractive, "interactive", "i", false, "Open live interactive log viewer")
	logCmd.Flags().StringVar(&logExport, "export", "", "Export format: csv")
	logCmd.Flags().IntVar(&logLimit, "limit", 0, "Maximum number of entries to return (0 = no limit)")
}

func runLog(cmd *cobra.Command, args []string) error {
	// Validate flag combinations.
	if logSince != "" && logDate != "" {
		return fmt.Errorf("log: --since and --date are mutually exclusive")
	}
	if logUntil != "" && logDate != "" {
		return fmt.Errorf("log: --until and --date are mutually exclusive")
	}
	if logFollow && logExport != "" {
		return fmt.Errorf("log: --follow and --export are mutually exclusive")
	}
	if logFollow && logUntil != "" {
		return fmt.Errorf("log: --follow and --until are mutually exclusive")
	}
	if logInteractive && logExport != "" {
		return fmt.Errorf("log: --interactive and --export are mutually exclusive")
	}
	if logInteractive && logFollow {
		return fmt.Errorf("log: --interactive and --follow are mutually exclusive")
	}
	if logInteractive && flags.JSON {
		return fmt.Errorf("log: --interactive cannot be used with --json")
	}
	if flags.JSON && logExport != "" {
		return fmt.Errorf("log: --json and --export are mutually exclusive")
	}
	if logLimit < 0 {
		return fmt.Errorf("log: --limit must be >= 0")
	}
	if logExport != "" && logExport != "csv" {
		return fmt.Errorf("log: unsupported export format %q (supported: csv)", logExport)
	}

	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("log: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("log: resolve repo root: %w", err)
	}

	db, err := store.OpenDataDB(absRoot)
	if err != nil {
		return fmt.Errorf("log: open data database: %w", err)
	}
	defer db.Close()

	if err := store.MigrateDataDB(db); err != nil {
		return fmt.Errorf("log: migrate data database: %w", err)
	}

	filter := store.LogFilter{
		File:    logFile,
		Allow:   logAllow,
		Deny:    logDeny,
		Granted: logGranted,
		Pass:    logPass,
		Agent:   logAgent,
		Limit:   logLimit,
	}

	// Time window: --date, --since, or default 24h.
	if logDate != "" {
		day, err := time.ParseInLocation("2006-01-02", logDate, time.Local)
		if err != nil {
			return fmt.Errorf("log: --date: invalid date %q (expected YYYY-MM-DD)", logDate)
		}
		filter.Since = day
		filter.Until = day.Add(24 * time.Hour)
	} else if logSince != "" {
		since, err := parseSinceDuration(logSince)
		if err != nil {
			return fmt.Errorf("log: --since: %w", err)
		}
		filter.Since = since
	} else if !logFollow {
		// Default: last 24 hours.
		filter.Since = time.Now().Add(-24 * time.Hour)
	}
	if logUntil != "" {
		until, err := parseUntilTime(logUntil)
		if err != nil {
			return fmt.Errorf("log: --until: %w", err)
		}
		filter.Until = until
	}
	if !filter.Since.IsZero() && !filter.Until.IsZero() && !filter.Since.Before(filter.Until) {
		return fmt.Errorf("log: invalid time window: --since must be earlier than --until")
	}

	if logFollow {
		// Follow mode is an unfiltered live stream of all event types.
		// Ignore category and attribute filters so policy, pass, allow, and deny
		// entries all appear in a single feed.
		filter.File = ""
		filter.Agent = ""
		filter.Allow = false
		filter.Deny = false
		filter.Granted = false
		filter.Pass = false
		filter.Since = time.Time{}
		filter.Until = time.Time{}
		return runLogFollow(db, filter)
	}
	if logInteractive {
		return tui.LiveLog(db, filter)
	}

	entries, err := store.ListUnifiedLog(db, filter)
	if err != nil {
		return fmt.Errorf("log: query: %w", err)
	}

	if flags.JSON {
		list := entries
		if list == nil {
			list = []store.UnifiedEntry{}
		}
		out, _ := json.MarshalIndent(map[string]any{"entries": list}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if logExport == "csv" {
		return writeLogCSV(os.Stdout, entries)
	}

	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no log entries")
		return nil
	}

	var buf bytes.Buffer
	for i, e := range entries {
		formatLogEntry(&buf, e)
		if i < len(entries)-1 {
			buf.WriteByte('\n')
		}
	}

	if isTTY(os.Stdout) {
		return pageOutput(buf.Bytes())
	}
	_, err = os.Stdout.Write(buf.Bytes())
	return err
}

// runLogFollow polls the database for new entries and streams them to stdout.
// It exits on SIGINT.
func runLogFollow(db *sql.DB, filter store.LogFilter) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Start from now if no --since was given.
	if filter.Since.IsZero() {
		filter.Since = time.Now()
	}

	// Print a starting marker so the user knows it's running.
	if !flags.JSON {
		fmt.Fprintf(os.Stderr, "Following log (ctrl-c to stop)...\n")
	}

	// Track the last-seen high-water mark to avoid re-printing entries.
	// The audit_log table uses RFC3339 timestamps with only second precision,
	// so advancing by microseconds doesn't prevent re-fetching the same row.
	// Instead we keep a set of recently seen entry fingerprints.
	seen := make(map[string]struct{})

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			entries, err := store.ListUnifiedLog(db, filter)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cordon: log follow: %v\n", err)
				continue
			}
			if len(entries) == 0 {
				continue
			}

			// Build the next seen set from this batch.
			nextSeen := make(map[string]struct{}, len(entries))

			// Entries come newest-first; print oldest-first for follow mode.
			for i := len(entries) - 1; i >= 0; i-- {
				e := entries[i]
				key := followEntryKey(e)
				nextSeen[key] = struct{}{}
				if _, dup := seen[key]; dup {
					continue
				}
				if flags.JSON {
					out, _ := json.Marshal(e)
					fmt.Println(string(out))
				} else {
					var buf bytes.Buffer
					formatLogEntry(&buf, e)
					os.Stdout.Write(buf.Bytes())
				}
			}

			seen = nextSeen

			// Advance the Since cursor to the newest entry's timestamp.
			// Entries within the same second will be deduped by the seen set.
			filter.Since = entries[0].Time
		}
	}
}

// followEntryKey returns a deduplication key for a unified log entry.
func followEntryKey(e store.UnifiedEntry) string {
	return e.Time.Format(time.RFC3339Nano) + "|" + e.EventType + "|" + e.ToolName + "|" + e.FilePath + "|" + e.Detail
}

// formatLogEntry writes a coloured entry to buf.
//
// Line 1: <BADGE>  [tool  ]  <subject>
// Line 2 (deny only): Reason: <reason text>
// Line 3: metadata line with timestamp, agent, session, pass, and detail.
func formatLogEntry(buf *bytes.Buffer, e store.UnifiedEntry) {
	const reset = "\033[0m"
	const dim = "\033[2m"

	label, color := logEventLabel(e.EventType)
	ts := formatTimestamp(e.Time)

	fmt.Fprintf(buf, "%s%-6s%s", color, label, reset)

	if e.ToolName != "" {
		fmt.Fprintf(buf, "  %-12s", e.ToolName)
	}

	subject := e.FilePath
	if subject == "" && e.Command != "" {
		subject = e.Command
		if len(subject) > 60 {
			subject = subject[:60] + "…"
		}
	}
	if subject != "" {
		fmt.Fprintf(buf, "  %s", subject)
	}
	buf.WriteByte('\n')

	if e.EventType == "hook_deny" && e.DeniedOpReason != "" {
		reason := e.DeniedOpReason
		var parts []string
		if e.MatchedRulePattern != "" {
			parts = append(parts, "rule: "+e.MatchedRulePattern)
		}
		if e.MatchedRuleType != "" {
			parts = append(parts, "type: "+e.MatchedRuleType)
		}
		if len(parts) > 0 {
			reason += " (" + strings.Join(parts, ", ") + ")"
		}
		fmt.Fprintf(buf, "        %sReason:%s %s\n", dim, reset, reason)
	}

	// Metadata line: <timestamp>  ·  <agent>  ·  <session>  ·  <pass>  ·  <detail>
	meta := []string{ts}
	if e.Agent != "" {
		meta = append(meta, e.Agent)
	}
	if e.SessionID != "" {
		meta = append(meta, "session: "+e.SessionID)
	}
	if e.PassID != "" {
		meta = append(meta, "pass: "+e.PassID)
	}
	if e.Detail != "" {
		meta = append(meta, e.Detail)
	}
	if len(meta) > 0 {
		fmt.Fprintf(buf, "        %s%s%s\n", dim, strings.Join(meta, "  ·  "), reset)
	}
}

// logEventLabel returns the display badge and ANSI colour for an event type.
func logEventLabel(eventType string) (label, color string) {
	const (
		boldRed = "\033[1;31m"
		red     = "\033[0;31m"
		green   = "\033[0;32m"
		yellow  = "\033[0;33m"
		cyan    = "\033[0;36m"
		dim     = "\033[2m"
	)
	switch eventType {
	case "hook_deny":
		return "DENY", boldRed
	case "hook_allow":
		return "ALLOW", green
	case "file_add":
		return "FILE+", yellow
	case "file_remove":
		return "FILE-", red
	case "pass_issue":
		return "PASS+", cyan
	case "pass_revoke":
		return "PASS-", red
	case "pass_expire":
		return "PASS!", dim
	default:
		return eventType, dim
	}
}

// isTTY reports whether f is an interactive terminal.
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// pageOutput pipes content to $PAGER (defaulting to less -RFX).
// Falls back to a direct write if the pager binary is not found.
func pageOutput(content []byte) error {
	pagerCmd := os.Getenv("PAGER")
	if pagerCmd == "" {
		pagerCmd = "less"
	}

	parts := strings.Fields(pagerCmd)
	var c *exec.Cmd
	if parts[0] == "less" {
		c = exec.Command("less", "-RFX")
	} else {
		c = exec.Command(parts[0], parts[1:]...)
	}
	c.Stdin = bytes.NewReader(content)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		// Pager unavailable — write directly.
		_, err2 := os.Stdout.Write(content)
		return err2
	}
	return nil
}

// writeLogCSV writes entries as CSV to w.
func writeLogCSV(w io.Writer, entries []store.UnifiedEntry) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"timestamp", "event_type", "tool_name", "file_path",
		"file_rule_id", "pass_id", "user", "agent", "session_id", "detail",
	}); err != nil {
		return err
	}
	for _, e := range entries {
		if err := cw.Write([]string{
			e.Time.UTC().Format(time.RFC3339),
			e.EventType,
			e.ToolName,
			e.FilePath,
			e.FileRuleID,
			e.PassID,
			e.User,
			e.Agent,
			e.SessionID,
			e.Detail,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// formatTimestamp returns a relative "Xh ago" / "Ym ago" string for entries
// within the last 24 hours, and an absolute timestamp otherwise.
func formatTimestamp(t time.Time) string {
	ago := time.Since(t)
	if ago < 0 || ago >= 24*time.Hour {
		return t.Local().Format("2006-01-02 15:04:05")
	}
	if ago < time.Minute {
		return "just now"
	}
	if ago < time.Hour {
		return fmt.Sprintf("%dm ago", int(ago.Minutes()))
	}
	h := int(ago.Hours())
	m := int(ago.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh ago", h)
	}
	return fmt.Sprintf("%dh%dm ago", h, m)
}

// parseSinceDuration parses a duration string into a time.Time representing
// "now minus that duration". Accepts standard Go durations (e.g. 24h, 90m)
// plus a day shorthand (e.g. 7d).
func parseSinceDuration(s string) (time.Time, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q", s)
		}
		return time.Now().Add(-time.Duration(n) * 24 * time.Hour), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return time.Now().Add(-d), nil
}

// parseUntilTime parses an absolute upper bound time.
// Accepted forms:
//   - RFC3339 timestamp (e.g. 2026-03-22T15:04:05Z)
//   - local date in YYYY-MM-DD, interpreted as the end of that day (exclusive).
func parseUntilTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if day, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return day.Add(24 * time.Hour), nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q (expected RFC3339 or YYYY-MM-DD)", s)
}
