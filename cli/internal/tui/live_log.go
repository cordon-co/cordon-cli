package tui

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/store"
	"golang.org/x/sys/unix"
)

const (
	livePollInterval = 2 * time.Second
	timeColWidth     = 16
)

type liveKey int

const (
	keyUnknown liveKey = iota
	keyQuit
	keyTogglePause
	keyUp
	keyDown
	keyRight
	keyLeft
)

// LiveLog runs an interactive live log viewer.
//
// Controls:
//   - p: toggle play/pause
//   - up/down: move selection
//   - right: expand selected row details
//   - left: collapse expanded details
//   - q or ctrl-c: quit
func LiveLog(db *sql.DB, filter store.LogFilter) error {
	fd := int(os.Stdin.Fd())
	if !isTerminal(fd) {
		return fmt.Errorf("log: interactive mode requires a TTY")
	}

	oldState, err := makeRaw(fd)
	if err != nil {
		return fmt.Errorf("log: interactive mode: set raw mode: %w", err)
	}
	defer restore(fd, oldState)

	fmt.Fprint(os.Stderr, "\033[?1049h\033[H")
	defer fmt.Fprint(os.Stderr, "\033[?1049l")
	hideCursor()
	defer showCursor()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	keyCh := make(chan liveKey, 16)
	go readLiveKeys(keyCh)

	entries, err := store.ListUnifiedLog(db, filter)
	if err != nil {
		return fmt.Errorf("log: query: %w", err)
	}

	state := liveState{
		paused:    false,
		entries:   entries,
		selected:  0,
		scrollRow: 0,
		expanded:  false,
		updatedAt: time.Now(),
	}

	ticker := time.NewTicker(livePollInterval)
	defer ticker.Stop()

	state.render()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if state.paused {
				continue
			}
			selectionKey := state.selectedKey()
			entries, err := store.ListUnifiedLog(db, filter)
			if err != nil {
				state.lastErr = err
				state.render()
				continue
			}
			state.lastErr = nil
			state.entries = entries
			state.updatedAt = time.Now()
			state.retargetSelection(selectionKey)
			state.render()
		case key := <-keyCh:
			switch key {
			case keyQuit:
				return nil
			case keyTogglePause:
				state.paused = !state.paused
				if !state.paused {
					entries, err := store.ListUnifiedLog(db, filter)
					if err != nil {
						state.lastErr = err
					} else {
						state.lastErr = nil
						state.entries = entries
						state.updatedAt = time.Now()
						if state.selected >= len(state.entries) {
							state.selected = maxInt(0, len(state.entries)-1)
						}
					}
				}
			case keyUp:
				state.moveSelection(-1)
			case keyDown:
				state.moveSelection(1)
			case keyRight:
				if len(state.entries) > 0 {
					state.expanded = true
				}
			case keyLeft:
				state.expanded = false
			}
			state.render()
		}
	}
}

type liveState struct {
	paused    bool
	entries   []store.UnifiedEntry
	selected  int
	scrollRow int
	expanded  bool
	updatedAt time.Time
	lastErr   error
}

func (s *liveState) moveSelection(delta int) {
	if len(s.entries) == 0 {
		s.selected = 0
		return
	}
	s.selected += delta
	if s.selected < 0 {
		s.selected = 0
	}
	if s.selected >= len(s.entries) {
		s.selected = len(s.entries) - 1
	}
}

func (s *liveState) selectedKey() string {
	if len(s.entries) == 0 || s.selected < 0 || s.selected >= len(s.entries) {
		return ""
	}
	return liveEntryKey(s.entries[s.selected])
}

func (s *liveState) retargetSelection(key string) {
	if len(s.entries) == 0 {
		s.selected = 0
		return
	}
	if key == "" {
		if s.selected >= len(s.entries) {
			s.selected = len(s.entries) - 1
		}
		return
	}
	for i, e := range s.entries {
		if liveEntryKey(e) == key {
			s.selected = i
			return
		}
	}
	if s.selected >= len(s.entries) {
		s.selected = len(s.entries) - 1
	}
}

func (s *liveState) render() {
	width, height := terminalSize()
	maxRows := maxInt(3, height-5)
	if s.selected < s.scrollRow {
		s.scrollRow = s.selected
	}
	if s.selected >= s.scrollRow+maxRows {
		s.scrollRow = s.selected - maxRows + 1
	}

	fmt.Fprint(os.Stderr, "\033[2J\033[H")

	toggleLabel := "pause"
	toggleIcon := "⏸"
	if s.paused {
		toggleLabel = "play "
		toggleIcon = "▶"
	}
	fmt.Fprintln(os.Stderr, trimToWidth(fmt.Sprintf("P %s %s | ↑↓ move | → details | ← collapse | q quit", toggleLabel, toggleIcon), width))
	fmt.Fprintln(os.Stderr, strings.Repeat("-", maxInt(1, width)))

	if len(s.entries) == 0 {
		fmt.Fprintln(os.Stderr, "No log entries yet.")
	} else {
		end := minInt(len(s.entries), s.scrollRow+maxRows)
		for i := s.scrollRow; i < end; i++ {
			e := s.entries[i]
			prefix := "  "
			if i == s.selected {
				prefix = "> "
			}
			line := fmt.Sprintf("%s%s %s", prefix, paddedLiveTime(e.Time), liveActionSummary(e))
			fmt.Fprintln(os.Stderr, trimToWidth(line, width))

			if s.expanded && i == s.selected {
				for _, detailLine := range detailGridLines(e, width) {
					fmt.Fprintln(os.Stderr, trimToWidth("    "+detailLine, width))
				}
			}
		}
	}

	if s.lastErr != nil {
		fmt.Fprintln(os.Stderr, strings.Repeat("-", maxInt(1, width)))
		errLine := "refresh error: " + s.lastErr.Error()
		fmt.Fprintln(os.Stderr, trimToWidth(errLine, width))
	}
}

func liveSubject(e store.UnifiedEntry) string {
	if e.FilePath != "" {
		return e.FilePath
	}
	if e.Command != "" {
		return cleanInline(e.Command)
	}
	if e.Detail != "" {
		return cleanInline(e.Detail)
	}
	return "-"
}

func liveActionSummary(e store.UnifiedEntry) string {
	agent := e.Agent
	if agent == "" {
		agent = "unknown-agent"
	}

	action := e.ToolName
	if action == "" {
		switch e.EventType {
		case "hook_allow":
			action = "write"
		case "hook_deny":
			action = "blocked_write"
		default:
			if e.EventType != "" {
				action = e.EventType
			} else {
				action = "event"
			}
		}
	}

	subject := liveSubject(e)
	if subject == "-" {
		return fmt.Sprintf("%s used %s", agent, action)
	}
	return fmt.Sprintf("%s used %s on file %s", agent, action, subject)
}

func paddedLiveTime(t time.Time) string {
	label := liveTimeLabel(t)
	if len(label) >= timeColWidth {
		return label
	}
	return label + strings.Repeat(" ", timeColWidth-len(label))
}

func liveTimeLabel(t time.Time) string {
	ago := time.Since(t)
	if ago < 0 {
		return t.Local().Format("15:04 02/01/2006")
	}
	if ago < time.Minute {
		return "now"
	}
	if ago < time.Hour {
		return fmt.Sprintf("%dm ago", int(ago.Minutes()))
	}
	if ago < 24*time.Hour {
		h := int(ago.Hours())
		m := int(ago.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh ago", h)
		}
		return fmt.Sprintf("%dh%dm ago", h, m)
	}
	return t.Local().Format("15:04 02/01/2006")
}

func safeTool(name string) string {
	if name == "" {
		return "-"
	}
	return name
}

func detailGridLines(e store.UnifiedEntry, width int) []string {
	type field struct {
		name  string
		value string
	}
	fields := make([]field, 0, 16)
	appendField := func(name, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		fields = append(fields, field{name: name, value: cleanInline(value)})
	}

	appendField("time", e.Time.Local().Format("2006-01-02 15:04:05"))
	appendField("event", e.EventType)
	appendField("tool", e.ToolName)
	appendField("file", e.FilePath)
	appendField("command", e.Command)
	if e.EventType == "hook_allow" {
		appendField("decision", "allow")
	}
	if e.EventType == "hook_deny" {
		appendField("decision", "deny")
	}
	appendField("reason", e.DeniedOpReason)
	appendField("rule pattern", e.MatchedRulePattern)
	appendField("rule type", e.MatchedRuleType)
	appendField("pass", e.PassID)
	appendField("agent", e.Agent)
	appendField("user", e.User)
	appendField("session", e.SessionID)
	appendField("detail", e.Detail)

	if len(fields) == 0 {
		return []string{"(no details)"}
	}

	colWidth := maxInt(20, (width-8)/2)
	lines := make([]string, 0, (len(fields)+1)/2)
	for i := 0; i < len(fields); i += 2 {
		left := fmt.Sprintf("%-11s %s", fields[i].name+":", fields[i].value)
		left = trimToWidth(left, colWidth)
		if i+1 >= len(fields) {
			lines = append(lines, left)
			continue
		}
		right := fmt.Sprintf("%-11s %s", fields[i+1].name+":", fields[i+1].value)
		right = trimToWidth(right, colWidth)
		lines = append(lines, fmt.Sprintf("%-*s  %s", colWidth, left, right))
	}
	return lines
}

func readLiveKeys(out chan<- liveKey) {
	buf := make([]byte, 8)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}
		if n == 0 {
			continue
		}
		for i := 0; i < n; i++ {
			b := buf[i]
			switch b {
			case 'q', 'Q':
				out <- keyQuit
			case 'p', 'P':
				out <- keyTogglePause
			case 3: // ctrl-c
				out <- keyQuit
			case 27:
				if i+2 < n && buf[i+1] == '[' {
					switch buf[i+2] {
					case 'A':
						out <- keyUp
					case 'B':
						out <- keyDown
					case 'C':
						out <- keyRight
					case 'D':
						out <- keyLeft
					}
					i += 2
				}
			}
		}
	}
}

func liveEntryKey(e store.UnifiedEntry) string {
	return e.Time.Format(time.RFC3339Nano) + "|" + e.EventType + "|" + e.ToolName + "|" + e.FilePath + "|" + e.Detail
}

func trimToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 1 {
		return string(r[:width])
	}
	return string(r[:width-1]) + "…"
}

func cleanInline(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

func terminalSize() (width, height int) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 {
		return 120, 32
	}
	return int(ws.Col), int(ws.Row)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
