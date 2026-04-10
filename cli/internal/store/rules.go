package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"
)

// StandardGuardrails is the default set of guardrail command rules offered during
// `cordon init`. These are stored as standard-authority deny rules in policy.db
// (not hardcoded like built-ins), so they are visible in `cordon command list`
// and can be removed if desired.
var StandardGuardrails = []string{
	// Destructive git operations
	"git reset --hard*",
	"git push *--force*",
	"git push *-f*",
	"git clean -f*",
	// Destructive filesystem operations
	"rm -rf /*",
	"rm -rf ~*",
	// .env file exposure — prevent agents reading secrets into context
	"cat .env",
	"cat .env.*",
	"cat */.env",
	"cat */.env.*",
}

// CommandRule is a policy rule that controls a shell command pattern.
type CommandRule struct {
	ID            string
	Pattern       string
	RuleType      string // "deny" (blocks command) or "allow" (permits command, overrides deny)
	RuleAuthority string // "standard" (any member) or "elevated" (elevated/admin only)
	CreatedBy     string
	CreatedAt     string
	UpdatedAt     string
	Notify        bool // triggers immediate sync when rule is matched
}

// AddRule inserts a command rule into the policy database.
// ruleAccess is "deny" (default) or "allow". ruleAuthority is "standard" or "elevated".
// Returns an error if the pattern already exists.
func AddRule(db *sql.DB, pattern, ruleAccess, ruleAuthority, createdBy string) (*CommandRule, error) {
	id, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("store: generate rule id: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)

	payload, _ := json.Marshal(map[string]interface{}{
		"id":             id,
		"pattern":        pattern,
		"rule_access":    ruleAccess,
		"rule_authority": ruleAuthority,
		"created_by":     createdBy,
		"created_at":     now,
		"updated_at":     now,
	})

	_, err = AppendEvent(db, "command_rule.added", string(payload), createdBy)
	if err != nil {
		if isDuplicatePatternError(err) {
			return nil, fmt.Errorf("store: add rule: %w: %s", ErrDuplicatePattern, pattern)
		}
		return nil, fmt.Errorf("store: add rule: %w", err)
	}

	return &CommandRule{
		ID:            id,
		Pattern:       pattern,
		RuleType:      ruleAccess,
		RuleAuthority: ruleAuthority,
		CreatedBy:     createdBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// ListRules returns all command rules ordered by creation time.
func ListRules(db *sql.DB) ([]CommandRule, error) {
	rows, err := db.Query(
		`SELECT id, pattern, rule_access, rule_authority, created_by, created_at, updated_at, notify
		 FROM command_rules ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list rules: %w", err)
	}
	defer rows.Close()

	var rules []CommandRule
	for rows.Next() {
		var r CommandRule
		var nfy int
		if err := rows.Scan(&r.ID, &r.Pattern, &r.RuleType, &r.RuleAuthority,
			&r.CreatedBy, &r.CreatedAt, &r.UpdatedAt, &nfy); err != nil {
			return nil, fmt.Errorf("store: scan rule: %w", err)
		}
		r.Notify = nfy != 0
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// RemoveRule deletes a standard-authority command rule with the given pattern.
// Returns (true, nil) if removed, (false, nil) if not found.
// Elevated-authority rules cannot be removed by non-elevated users.
func RemoveRule(db *sql.DB, pattern string) (bool, error) {
	// Look up the rule ID, enforcing standard-authority restriction.
	var id string
	err := db.QueryRow(
		`SELECT id FROM command_rules WHERE pattern = ? AND rule_authority = 'standard'`, pattern,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: remove rule lookup: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"id":      id,
		"pattern": pattern,
	})

	_, err = AppendEvent(db, "command_rule.removed", string(payload), CurrentOSUser())
	if err != nil {
		return false, fmt.Errorf("store: remove rule: %w", err)
	}
	return true, nil
}

// MatchCommandRule checks whether command matches any rule in the database.
// Returns the effective deny rule, or nil if the command is permitted.
//
// Allow rules supersede deny rules: if any matching rule has RuleType "allow",
// the command is considered permitted and nil is returned. If only deny rules
// match, the first matching deny rule is returned.
func MatchCommandRule(db *sql.DB, command string) (*CommandRule, error) {
	rules, err := ListRules(db)
	if err != nil {
		return nil, err
	}

	var firstDeny *CommandRule
	for _, r := range rules {
		if !commandMatchesPattern(command, r.Pattern) {
			continue
		}
		if r.RuleType == "allow" {
			return nil, nil // allow supersedes all deny rules
		}
		if firstDeny == nil {
			rCopy := r
			firstDeny = &rCopy
		}
	}
	return firstDeny, nil
}

// commandMatchesPattern reports whether command matches a rule pattern.
// Matching is additive:
//   - string matcher (exact/glob/prefix),
//   - argv matcher for option-sensitive patterns.
func commandMatchesPattern(command, pattern string) bool {
	if commandMatchesString(command, pattern) {
		return true
	}
	return commandMatchesArgv(command, pattern)
}

// commandMatchesString applies legacy string semantics:
// exact match, full-string glob match, and plain-prefix match.
// This is command-pattern matching logic (not file-rule path matching).
func commandMatchesString(command, pattern string) bool {
	command = strings.TrimSpace(command)
	pattern = canonicalCommandPattern(strings.TrimSpace(pattern))

	// Exact match.
	if command == pattern {
		return true
	}

	// path.Match provides shell-style glob matching on plain strings and avoids
	// OS-specific filepath separator behavior from path/filepath.
	matched, err := path.Match(pattern, command)
	if err != nil {
		// Invalid pattern — treat as no match.
		matched = false
	}
	if matched {
		return true
	}

	// For plain patterns without glob metacharacters, treat the pattern as a
	// command prefix so "echo" matches "echo hello" and "git push" matches
	// "git push origin main".
	if !hasGlobMeta(pattern) {
		if strings.HasPrefix(command, pattern+" ") {
			return true
		}
	}

	return false
}

// canonicalCommandPattern normalizes simple command-prefix patterns so
// "<cmd>" and "<cmd> *" are treated equivalently for matching.
// This only applies when the trailing "*" is the sole glob metacharacter.
func canonicalCommandPattern(pattern string) string {
	if !strings.HasSuffix(pattern, " *") {
		return pattern
	}
	base := strings.TrimSpace(strings.TrimSuffix(pattern, " *"))
	if base == "" {
		return pattern
	}
	if hasGlobMeta(base) {
		return pattern
	}
	return base
}

var globMetaRegex = regexp.MustCompile(`[*?\[]`)

func hasGlobMeta(pattern string) bool {
	return globMetaRegex.MatchString(pattern)
}

// commandMatchesArgv matches a command by split shell tokens with
// order-insensitive option matching. It is intentionally conservative and only
// engages when pattern contains at least one option token and no positional
// tokens after the first option token.
func commandMatchesArgv(command, pattern string) bool {
	cmdTokens, ok := shellCommandTokens(command)
	if !ok || len(cmdTokens) == 0 {
		return false
	}
	patTokens, ok := shellCommandTokens(pattern)
	if !ok || len(patTokens) == 0 {
		return false
	}

	firstOpt := -1
	for i, tok := range patTokens {
		if isOptionToken(tok) {
			firstOpt = i
			break
		}
	}
	if firstOpt < 0 {
		return false
	}
	// To avoid surprising behavior drift, only use argv option matching for
	// patterns whose suffix is option-only (e.g. "git push --force*").
	for _, tok := range patTokens[firstOpt:] {
		if !isOptionToken(tok) {
			return false
		}
	}

	// Prefix argv tokens (e.g. "git push") must match in order.
	if len(cmdTokens) < firstOpt {
		return false
	}
	for i := 0; i < firstOpt; i++ {
		if !tokenGlobMatch(patTokens[i], cmdTokens[i]) {
			return false
		}
	}

	// Option argv tokens match anywhere in the command tail.
	tail := cmdTokens[firstOpt:]
	for _, optPat := range patTokens[firstOpt:] {
		found := false
		for _, tok := range tail {
			if tokenGlobMatch(optPat, tok) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func tokenGlobMatch(patternTok, token string) bool {
	if patternTok == token {
		return true
	}
	matched, err := path.Match(patternTok, token)
	return err == nil && matched
}

func isOptionToken(tok string) bool {
	return strings.HasPrefix(tok, "-")
}

// shellCommandTokens parses a shell command segment and returns its call tokens
// as printed shell words. Returns false if parsing fails or no call expression
// can be found.
func shellCommandTokens(command string) ([]string, bool) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, false
	}

	var call *syntax.CallExpr
	syntax.Walk(file, func(node syntax.Node) bool {
		if node == nil || call != nil {
			return true
		}
		if c, ok := node.(*syntax.CallExpr); ok {
			call = c
			return false
		}
		return true
	})
	if call == nil {
		return nil, false
	}

	printer := syntax.NewPrinter()
	tokens := make([]string, 0, len(call.Args))
	for _, w := range call.Args {
		var buf bytes.Buffer
		if err := printer.Print(&buf, w); err != nil {
			return nil, false
		}
		tok := strings.TrimSpace(buf.String())
		if tok != "" {
			tokens = append(tokens, tok)
		}
	}
	if len(tokens) == 0 {
		return nil, false
	}
	return tokens, true
}
