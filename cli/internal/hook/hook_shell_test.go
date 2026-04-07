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

	cmdChecker := func(command, cwd string) (bool, string, *MatchedRule, bool) {
		if strings.TrimSpace(command) == "git status" {
			return false, "", &MatchedRule{Pattern: "git status", RuleType: "deny", RuleAuthority: "standard"}, false
		}
		return true, "", nil, false
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	event, err := Evaluate(strings.NewReader(payload), &out, &errOut, nil, nil, cmdChecker, "")
	if err != ErrDenied {
		t.Fatalf("Evaluate error = %v, want ErrDenied", err)
	}
	if event == nil {
		t.Fatal("event = nil, want non-nil deny event")
	}
	if event.Decision != DecisionDeny {
		t.Fatalf("event.Decision = %q, want %q", event.Decision, DecisionDeny)
	}
	if event.DeniedOpReason == "" {
		t.Fatal("event.DeniedOpReason is empty, want populated reason")
	}
	if event.DeniedOpReason != "prevent-command rule violation" {
		t.Fatalf("event.DeniedOpReason = %q, want prevent-command rule violation", event.DeniedOpReason)
	}
}

func TestEvaluate_RunInTerminalUsesCwdAwareReadChecks(t *testing.T) {
	payload := `{
  "tool_name": "run_in_terminal",
  "tool_input": {"command":"cd scripts && cat README.md"},
  "cwd": "/repo"
}`

	rdChecker := func(filePath, cwd string) (bool, string, bool) {
		if filePath == "/repo/scripts/README.md" {
			return false, "", false
		}
		return true, "", false
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	event, err := Evaluate(strings.NewReader(payload), &out, &errOut, nil, rdChecker, nil, "")
	if err != ErrDenied {
		t.Fatalf("Evaluate error = %v, want ErrDenied", err)
	}
	if event == nil {
		t.Fatal("event = nil, want non-nil deny event")
	}
	if event.FilePath != "/repo/scripts/README.md" {
		t.Fatalf("event.FilePath = %q, want /repo/scripts/README.md", event.FilePath)
	}
}

func TestEvaluate_RunInTerminalRecordsCommandPass(t *testing.T) {
	payload := `{
  "tool_name": "run_in_terminal",
  "tool_input": {"command":"git push --force origin main"},
  "cwd": "/repo"
}`

	cmdChecker := func(command, cwd string) (bool, string, *MatchedRule, bool) {
		if strings.TrimSpace(command) == "git push --force origin main" {
			return true, "pass-cmd-123", nil, true
		}
		return true, "", nil, false
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	event, err := Evaluate(strings.NewReader(payload), &out, &errOut, nil, nil, cmdChecker, "")
	if err != nil {
		t.Fatalf("Evaluate error = %v, want nil", err)
	}
	if event == nil {
		t.Fatal("event = nil, want non-nil allow event")
	}
	if event.Decision != DecisionAllow {
		t.Fatalf("event.Decision = %q, want %q", event.Decision, DecisionAllow)
	}
	if event.PassID != "pass-cmd-123" {
		t.Fatalf("event.PassID = %q, want pass-cmd-123", event.PassID)
	}
	if !event.Notify {
		t.Fatal("event.Notify = false, want true")
	}
}
