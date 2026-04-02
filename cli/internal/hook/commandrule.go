package hook

import (
	"bytes"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// MatchedRule describes a command rule that was matched against a command.
type MatchedRule struct {
	Pattern       string
	RuleType      string // "deny" or "allow"
	RuleAuthority string // "standard" or "elevated"
}

// CommandChecker checks whether a bash command segment is allowed by command rules.
//
// command is a single (already-split) shell command segment.
// cwd is the agent working directory used to locate the policy database.
//
// Return values:
//   - true,  nil, false   — command is allowed
//   - false, rule, notify — command is blocked; rule describes the matching rule
//
// A nil CommandChecker allows all commands (fail-open).
type CommandChecker func(command, cwd string) (allowed bool, matched *MatchedRule, notify bool)

// builtinRule is a command rule compiled into the binary.
type builtinRule struct {
	Pattern  string
	RuleType string // "deny" or "allow"
}

// builtinRules are always active regardless of policy.db contents.
// These protect the Cordon system itself and cover SAF-01 destructive operations.
// Allow rules (e.g. "cordon hook") supersede deny rules, just like DB rules.
var builtinRules = []builtinRule{
	// Deny: agents must not run cordon CLI commands.
	{Pattern: "cordon", RuleType: "deny"},
	{Pattern: "cordon *", RuleType: "deny"},
	// Allow: the hook runner is invoked by the agent framework, not the agent.
	{Pattern: "cordon hook", RuleType: "allow"},
}

// CheckBuiltinRules checks command against all built-in rules.
// Returns the first matching deny rule, or nil if the command is permitted.
// Allow rules supersede deny rules: if any built-in allow rule matches,
// the command is permitted regardless of deny rules.
func CheckBuiltinRules(command string) *MatchedRule {
	command = strings.TrimSpace(command)
	var firstDeny *MatchedRule
	for _, r := range builtinRules {
		if !commandMatchesBuiltin(command, r.Pattern) {
			continue
		}
		if r.RuleType == "allow" {
			return nil // allow supersedes all deny rules
		}
		if firstDeny == nil {
			firstDeny = &MatchedRule{
				Pattern:       r.Pattern,
				RuleType:      "deny",
				RuleAuthority: "elevated",
			}
		}
	}
	return firstDeny
}

// commandMatchesBuiltin reports whether command matches a built-in pattern.
func commandMatchesBuiltin(command, pattern string) bool {
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
	command = strings.TrimSpace(command)
	if command == "" {
		return []string{""}
	}

	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		// Fall back to the raw command if parsing fails.
		return []string{command}
	}

	printer := syntax.NewPrinter()
	var segments []string
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}

		var buf bytes.Buffer
		if err := printer.Print(&buf, call); err != nil {
			return true
		}

		seg := strings.TrimSpace(buf.String())
		if seg != "" {
			segments = append(segments, seg)
		}
		return true
	})

	if len(segments) == 0 {
		// Commands with no call expressions (e.g. bare assignments) still need
		// to be matched against command rules as a full string.
		return []string{command}
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
