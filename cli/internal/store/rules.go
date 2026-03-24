package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// StandardGuardrails is the default set of guardrail command rules offered during
// `cordon init`. These are stored as standard-authority deny rules in policy.db
// (not hardcoded like built-ins), so they are visible in `cordon command list`
// and can be removed if desired.
var StandardGuardrails = []string{
	// Destructive git operations
	"git reset --hard*",
	"git push --force*",
	"git push -f*",
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
