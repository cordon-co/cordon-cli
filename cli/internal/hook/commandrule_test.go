package hook

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitCompoundCommand_QuotedDelimiters(t *testing.T) {
	got := splitCompoundCommand(`echo "a && b ; c | d" && git status`)
	want := []string{`echo "a && b ; c | d"`, "git status"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
}

func TestSplitCompoundCommand_NestedAndPipeline(t *testing.T) {
	got := splitCompoundCommand(`cd /tmp && (git status; git add a.txt) | cat && echo done`)
	want := []string{"cd /tmp", "git status", "git add a.txt", "cat", "echo done"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
}

func TestSplitCompoundCommand_ParseFailureFallsBackToRaw(t *testing.T) {
	got := splitCompoundCommand(`echo "unterminated`)
	want := []string{`echo "unterminated`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
}

func TestSplitCompoundCommand_NoCallExpressionFallsBackToRaw(t *testing.T) {
	got := splitCompoundCommand(`FOO=bar`)
	want := []string{"FOO=bar"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
}

func TestCommandRuleDenyReason_IncludesPassGuidance(t *testing.T) {
	reason := commandRuleDenyReason(&MatchedRule{Pattern: "echo", RuleType: "deny"}, "claude-code")
	if !strings.Contains(reason, "cordon pass issue <target>") {
		t.Fatalf("reason missing pass guidance: %q", reason)
	}
	if !strings.Contains(reason, "Cordon command rule") {
		t.Fatalf("reason missing command rule wording: %q", reason)
	}
	if !strings.Contains(reason, "cordon_request_access MCP tool") {
		t.Fatalf("reason missing mcp guidance for supported agent: %q", reason)
	}
}

func TestCommandRuleDenyReason_OmitsMCPForGeminiAndOpenCode(t *testing.T) {
	agents := []string{"gemini", "gemini-cli", "opencode"}
	for _, agent := range agents {
		t.Run(agent, func(t *testing.T) {
			reason := commandRuleDenyReason(&MatchedRule{Pattern: "echo", RuleType: "deny"}, agent)
			if strings.Contains(reason, "cordon_request_access MCP tool") {
				t.Fatalf("reason should not mention MCP for %s: %q", agent, reason)
			}
			if !strings.Contains(reason, "ask the user to grant access themselves using the command cordon pass issue <target>") {
				t.Fatalf("reason missing ask-user guidance for %s: %q", agent, reason)
			}
			if !strings.Contains(reason, "You should ask the user for a pass.") {
				t.Fatalf("reason missing ask-user-only sentence for %s: %q", agent, reason)
			}
		})
	}
}

func TestCheckBuiltinRules_DenyCordonCommands(t *testing.T) {
	rule := CheckBuiltinRules("cordon status")
	if rule == nil {
		t.Fatal("expected built-in deny rule for cordon status, got nil")
	}
	if rule.Pattern != "cordon *" {
		t.Fatalf("rule.Pattern = %q, want %q", rule.Pattern, "cordon *")
	}
}

func TestCheckBuiltinRules_AllowOverridesDenyForCordonHook(t *testing.T) {
	rule := CheckBuiltinRules("cordon hook")
	if rule != nil {
		t.Fatalf("expected built-in allow override for cordon hook, got deny rule %q", rule.Pattern)
	}
}
