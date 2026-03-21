package hook

import (
	"fmt"
	"strings"

	"github.com/cordon-co/cordon/internal/store"
)

// MatchedRule describes a command rule that was matched against a command.
type MatchedRule struct {
	Pattern       string
	RuleType      string // "deny" or "allow"
	RuleAuthority string // "standard" or "guardian"
}

// CommandChecker checks whether a bash command segment is allowed by command rules.
//
// command is a single (already-split) shell command segment.
// cwd is the agent working directory used to locate the policy database.
//
// Return values:
//   - true,  nil   — command is allowed
//   - false, rule  — command is blocked; rule describes the matching rule
//
// A nil CommandChecker allows all commands (fail-open).
type CommandChecker func(command, cwd string) (allowed bool, matched *MatchedRule)

// builtinRules are always active regardless of policy.db contents.
// These protect the Cordon system itself and cover SAF-01 destructive operations.
var builtinRules = []string{
	"cordon",
	"cordon *",
}

// BuiltinRulesAsStore returns the built-in rules as store.CommandRule values
// for display in `cordon command list`.
func BuiltinRulesAsStore() []store.CommandRule {
	rules := make([]store.CommandRule, len(builtinRules))
	for i, pattern := range builtinRules {
		rules[i] = store.CommandRule{
			Pattern:       pattern,
			RuleType:      "deny",
			RuleAuthority: "guardian",
		}
	}
	return rules
}

// CheckBuiltinRules checks command against all built-in rules.
// Returns the first matching rule, or nil if none match.
func CheckBuiltinRules(command string) *MatchedRule {
	command = strings.TrimSpace(command)
	for _, pattern := range builtinRules {
		if commandMatchesBuiltin(command, pattern) {
			return &MatchedRule{
				Pattern:       pattern,
				RuleType:      "deny",
				RuleAuthority: "guardian",
			}
		}
	}
	return nil
}

// commandMatchesBuiltin reports whether command matches a built-in pattern.
// "cordon hook" is always exempt — it is the hook runner invoked by the
// agent framework itself, not a command issued by the agent.
func commandMatchesBuiltin(command, pattern string) bool {
	// Exempt the hook runner from all cordon-related built-in rules.
	if command == "cordon hook" || strings.HasPrefix(command, "cordon hook ") {
		return false
	}
	if command == pattern {
		return true
	}
	// Simple prefix match for "cordon *" style patterns ending with " *".
	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, " *")
		return strings.HasPrefix(command, prefix+" ") || command == prefix
	}
	return false
}

// splitCompoundCommand splits a compound shell command into individual segments.
// Splits on: &&, ||, ;, and | (pipe).
// Each segment is trimmed of leading/trailing whitespace.
func splitCompoundCommand(command string) []string {
	// Replace all compound operators with a common delimiter.
	// Process longest tokens first to avoid partial matches.
	s := command
	s = strings.ReplaceAll(s, "&&", "\x00")
	s = strings.ReplaceAll(s, "||", "\x00")
	s = strings.ReplaceAll(s, ";", "\x00")
	s = strings.ReplaceAll(s, "|", "\x00")

	parts := strings.Split(s, "\x00")
	var segments []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			segments = append(segments, p)
		}
	}
	if len(segments) == 0 {
		return []string{strings.TrimSpace(command)}
	}
	return segments
}

// commandRuleDenyReason returns the denial message for a matched command rule.
func commandRuleDenyReason(rule *MatchedRule) string {
	return fmt.Sprintf(
		"CORDON POLICY: This command is prohibited by a Cordon command rule. "+
			"Rule: %s. "+
			"This is an enforced policy restriction, not a technical error.",
		rule.Pattern,
	)
}
