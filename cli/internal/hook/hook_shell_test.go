package hook

import (
	"bytes"
	"strings"
	"testing"
)

func TestIsShellCommandTool(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "bash", in: "bash", want: true},
		{name: "Bash", in: "Bash", want: true},
		{name: "run in terminal", in: "run_in_terminal", want: true},
		{name: "other", in: "read_file", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isShellCommandTool(tt.in); got != tt.want {
				t.Fatalf("isShellCommandTool(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestEvaluate_RunInTerminalAppliesCommandRules(t *testing.T) {
	payload := `{
  "tool_name": "run_in_terminal",
  "tool_input": {"command":"cd /Users/tom/Projects/cordon && git status"},
  "cwd": "/Users/tom/Projects/cordon"
}`

	cmdChecker := func(command, cwd string) (bool, *MatchedRule, bool) {
		if strings.TrimSpace(command) == "git status" {
			return false, &MatchedRule{Pattern: "git status", RuleType: "deny", RuleAuthority: "standard"}, false
		}
		return true, nil, false
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	event, err := Evaluate(strings.NewReader(payload), &out, &errOut, nil, nil, cmdChecker)
	if err != ErrDenied {
		t.Fatalf("Evaluate error = %v, want ErrDenied", err)
	}
	if event == nil {
		t.Fatal("event = nil, want non-nil deny event")
	}
	if event.Decision != DecisionDeny {
		t.Fatalf("event.Decision = %q, want %q", event.Decision, DecisionDeny)
	}
}
