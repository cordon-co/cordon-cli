// Package tui provides interactive terminal UI components using raw ANSI
// escape sequences. No external TUI dependencies.
package tui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

// Option represents one item in the checkbox list.
type Option struct {
	Label      string // display text
	Selectable bool   // false for "coming soon" items
	Selected   bool   // pre-selected state
	Suffix     string // e.g. "(coming soon)" displayed in dim
}

// ErrAborted is returned when the user aborts the selection (Ctrl-C or q).
var ErrAborted = fmt.Errorf("selection aborted")

// MultiSelect displays an interactive checkbox list and returns the indices
// of selected options. It reads from os.Stdin and writes ANSI escape sequences
// to os.Stderr (so stdout remains clean for --json).
//
// Controls: arrow keys navigate, space toggles, enter confirms.
// Non-selectable items are displayed but cannot be toggled.
//
// If stdin is not a terminal, returns all selectable options pre-selected.
func MultiSelect(prompt string, options []Option) ([]int, error) {
	fd := int(os.Stdin.Fd())
	if !isTerminal(fd) {
		return allSelectable(options), nil
	}

	// Save terminal state and set raw mode.
	oldState, err := makeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("tui: set raw mode: %w", err)
	}
	defer restore(fd, oldState)

	// Restore terminal on SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		restore(fd, oldState)
		showCursor()
		os.Exit(130)
	}()

	cursor := firstSelectable(options)
	selected := make([]bool, len(options))
	for i, opt := range options {
		selected[i] = opt.Selected
	}

	w := os.Stderr
	hideCursor()
	defer showCursor()

	render(w, prompt, options, selected, cursor)

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return nil, err
		}

		switch {
		case n == 1 && buf[0] == '\r': // Enter
			return collectSelected(selected), nil
		case n == 1 && buf[0] == '\n': // Enter (some terminals)
			return collectSelected(selected), nil
		case n == 1 && buf[0] == ' ': // Space — toggle
			if options[cursor].Selectable {
				selected[cursor] = !selected[cursor]
			}
		case n == 1 && buf[0] == 3: // Ctrl-C
			return nil, ErrAborted
		case n == 1 && buf[0] == 'q':
			return nil, ErrAborted
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'A': // Up
			cursor = prevOption(options, cursor)
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'B': // Down
			cursor = nextOption(options, cursor)
		case n == 1 && buf[0] == 'k': // vim up
			cursor = prevOption(options, cursor)
		case n == 1 && buf[0] == 'j': // vim down
			cursor = nextOption(options, cursor)
		default:
			continue
		}

		// Move cursor up to re-render.
		for i := 0; i < len(options); i++ {
			fmt.Fprintf(w, "\033[A")
		}
		render(w, prompt, options, selected, cursor)
	}
}

func render(w *os.File, prompt string, options []Option, selected []bool, cursor int) {
	_ = prompt // prompt is printed once before calling MultiSelect
	for i, opt := range options {
		// Clear line
		fmt.Fprintf(w, "\033[2K")

		prefix := "  "
		if i == cursor {
			prefix = "\033[36m>\033[0m "
		}

		check := "[ ]"
		if selected[i] {
			check = "\033[32m[x]\033[0m"
		}

		label := opt.Label
		suffix := ""
		if opt.Suffix != "" {
			suffix = " \033[2m" + opt.Suffix + "\033[0m"
		}

		if !opt.Selectable {
			// Dim the entire line for non-selectable items.
			fmt.Fprintf(w, "%s\033[2m%s %s%s\033[0m\n", prefix, check, label, suffix)
		} else if i == cursor {
			// Bold the label for the highlighted item.
			fmt.Fprintf(w, "%s%s \033[1m%s\033[0m%s\n", prefix, check, label, suffix)
		} else {
			fmt.Fprintf(w, "%s%s %s%s\n", prefix, check, label, suffix)
		}
	}
}

func hideCursor() {
	fmt.Fprintf(os.Stderr, "\033[?25l")
}

func showCursor() {
	fmt.Fprintf(os.Stderr, "\033[?25h")
}

func firstSelectable(options []Option) int {
	for i, opt := range options {
		if opt.Selectable {
			return i
		}
	}
	return 0
}

func nextOption(options []Option, current int) int {
	for i := current + 1; i < len(options); i++ {
		return i
	}
	return current
}

func prevOption(options []Option, current int) int {
	for i := current - 1; i >= 0; i-- {
		return i
	}
	return current
}

func collectSelected(selected []bool) []int {
	var result []int
	for i, s := range selected {
		if s {
			result = append(result, i)
		}
	}
	return result
}

func allSelectable(options []Option) []int {
	var result []int
	for i, opt := range options {
		if opt.Selectable {
			result = append(result, i)
		}
	}
	return result
}

func isTerminal(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	return err == nil
}

func makeRaw(fd int) (*unix.Termios, error) {
	termios, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		return nil, err
	}

	oldState := *termios

	// Set raw mode: disable echo, canonical mode, signals, and input processing.
	termios.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN
	termios.Iflag &^= unix.IXON | unix.ICRNL | unix.BRKINT | unix.INPCK | unix.ISTRIP
	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0

	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, termios); err != nil {
		return nil, err
	}

	return &oldState, nil
}

func restore(fd int, state *unix.Termios) {
	_ = unix.IoctlSetTermios(fd, ioctlWriteTermios, state)
}
