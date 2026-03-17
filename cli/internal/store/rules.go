package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// StandardGuardrails is the default set of guardrail command rules offered during
// `cordon init`. These are stored as custom rules in policy.db (not hardcoded like
// built-ins), so they are visible in `cordon rule list` and can be removed if desired.
// In future they will be seeded from user directory config (~/.cordon/config).
var StandardGuardrails = []struct {
	Pattern string
	Reason  string
}{
	// Destructive git operations
	{"git reset --hard*", "Destructive"},
	{"git push --force*", "Destructive"},
	{"git push -f*", "Destructive"},
	{"git clean -f*", "Destructive"},
	// Destructive filesystem operations
	{"rm -rf /*", "Destructive"},
	{"rm -rf ~*", "Destructive"},
	// .env file exposure — prevent agents reading secrets into context
	{"cat .env", "Secret exposure"},
	{"cat .env.*", "Secret exposure"},
	{"cat */.env", "Secret exposure"},
	{"cat */.env.*", "Secret exposure"},
}

// CommandRule is a policy rule that blocks a shell command pattern.
type CommandRule struct {
	ID        string
	Pattern   string
	RuleType  string // "builtin" or "custom"
	Reason    string
	CreatedBy string
	CreatedAt string
	UpdatedAt string
}

// AddRule inserts a custom command rule into the policy database.
// Returns an error if the pattern already exists.
func AddRule(db *sql.DB, pattern, reason, createdBy string) (*CommandRule, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	id, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("store: generate rule id: %w", err)
	}

	r := CommandRule{
		ID:        id,
		Pattern:   pattern,
		RuleType:  "custom",
		Reason:    reason,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = db.Exec(
		`INSERT INTO command_rules (id, pattern, rule_type, reason, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Pattern, r.RuleType, r.Reason, r.CreatedBy, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("store: add rule: %w", err)
	}
	return &r, nil
}

// ListRules returns all custom command rules ordered by creation time.
func ListRules(db *sql.DB) ([]CommandRule, error) {
	rows, err := db.Query(
		`SELECT id, pattern, rule_type, reason, created_by, created_at, updated_at
		 FROM command_rules ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list rules: %w", err)
	}
	defer rows.Close()

	var rules []CommandRule
	for rows.Next() {
		var r CommandRule
		if err := rows.Scan(&r.ID, &r.Pattern, &r.RuleType, &r.Reason,
			&r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan rule: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// RemoveRule deletes the custom command rule with the given pattern.
// Returns (true, nil) if removed, (false, nil) if not found.
// Built-in rules cannot be removed.
func RemoveRule(db *sql.DB, pattern string) (bool, error) {
	res, err := db.Exec(
		`DELETE FROM command_rules WHERE pattern = ? AND rule_type = 'custom'`, pattern,
	)
	if err != nil {
		return false, fmt.Errorf("store: remove rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: remove rule rows affected: %w", err)
	}
	return n > 0, nil
}

// MatchCommandRule checks whether command matches any rule in the database.
// Returns the first matching rule, or nil if no rule matches.
func MatchCommandRule(db *sql.DB, command string) (*CommandRule, error) {
	rules, err := ListRules(db)
	if err != nil {
		return nil, err
	}
	for _, r := range rules {
		if commandMatchesPattern(command, r.Pattern) {
			rCopy := r
			return &rCopy, nil
		}
	}
	return nil, nil
}

// commandMatchesPattern reports whether command matches the glob-style pattern.
// The pattern is matched against the full command string. The special token *
// matches any sequence of characters.
func commandMatchesPattern(command, pattern string) bool {
	command = strings.TrimSpace(command)
	pattern = strings.TrimSpace(pattern)

	// Exact match.
	if command == pattern {
		return true
	}

	// filepath.Match uses the same glob syntax we want.
	matched, err := filepath.Match(pattern, command)
	if err != nil {
		// Invalid pattern — treat as no match.
		return false
	}
	return matched
}
