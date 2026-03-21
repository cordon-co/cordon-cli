package cmd

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var logFile string
var logDeniedOnly bool
var logSince string
var logExport string

// logCmd — named logCmd to avoid shadowing the standard library "log" package.
var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Display the audit log",
	Args:  cobra.NoArgs,
	RunE:  runLog,
}

func init() {
	logCmd.Flags().StringVar(&logFile, "file", "", "Filter by file path (substring match)")
	logCmd.Flags().BoolVar(&logDeniedOnly, "denied-only", false, "Show only denied hook operations")
	logCmd.Flags().StringVar(&logSince, "since", "", "Show entries newer than duration (e.g. 24h, 7d, 90m)")
	logCmd.Flags().StringVar(&logExport, "export", "", "Export format: csv")
}

func runLog(cmd *cobra.Command, args []string) error {
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
		File:       logFile,
		DeniedOnly: logDeniedOnly,
	}
	if logSince != "" {
		since, err := parseSinceDuration(logSince)
		if err != nil {
			return fmt.Errorf("log: --since: %w", err)
		}
		filter.Since = since
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

// formatLogEntry writes a two-line coloured entry to buf.
//
// Line 1:  <BADGE>  <timestamp>  [tool  ]  <subject>
// Line 2:           user: …  ·  agent: …  ·  detail
func formatLogEntry(buf *bytes.Buffer, e store.UnifiedEntry) {
	const reset = "\033[0m"
	const dim = "\033[2m"

	label, color := logEventLabel(e.EventType)
	ts := e.Time.Local().Format("2006-01-02 15:04:05")

	fmt.Fprintf(buf, "%s%-6s%s  %s", color, label, reset, ts)

	if e.ToolName != "" {
		fmt.Fprintf(buf, "  %-12s", e.ToolName)
	}

	subject := e.FilePath
	if subject != "" {
		fmt.Fprintf(buf, "  %s", subject)
	}
	buf.WriteByte('\n')

	// Metadata line.
	var meta []string
	if e.User != "" {
		meta = append(meta, "user: "+e.User)
	}
	if e.Agent != "" {
		meta = append(meta, "agent: "+e.Agent)
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
		"file_rule_id", "pass_id", "user", "agent", "detail",
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
			e.Detail,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
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
