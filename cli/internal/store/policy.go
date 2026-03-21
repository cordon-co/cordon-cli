package store

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ErrDuplicatePattern is returned when adding a file rule or command rule with
// a pattern that already exists in the database.
var ErrDuplicatePattern = errors.New("pattern already exists")

// isDuplicatePatternError reports whether err is a SQLite UNIQUE constraint
// violation on the pattern column.
func isDuplicatePatternError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed")
}

// FileRule is a protected file, folder, or glob pattern stored in policy.db.
type FileRule struct {
	ID            string
	Pattern       string
	FileType      string // "deny" (blocks access) or "allow" (permits access, overrides deny)
	FileAuthority string // "standard" (any member) or "guardian" (guardian/admin only)
	PreventWrite  bool   // always true for now
	PreventRead   bool   // opt-in via --prevent-read
	CreatedBy     string
	CreatedAt     string // ISO 8601
	UpdatedAt     string // ISO 8601
}

// AddFileRule inserts a new file rule into the policy database.
// fileAccess is "deny" (default) or "allow". fileAuthority is "standard" or "guardian".
// preventRead enables read enforcement in addition to the always-on write enforcement.
// Returns an error if the pattern already exists (UNIQUE constraint violation),
// or if fileAccess is "allow" and preventRead is true (nonsensical combination).
func AddFileRule(db *sql.DB, pattern, fileAccess, fileAuthority, createdBy string, preventRead bool) (*FileRule, error) {
	if fileAccess == "allow" && preventRead {
		return nil, fmt.Errorf("store: allow file rules cannot have prevent-read enabled")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("store: generate file rule id: %w", err)
	}

	f := FileRule{
		ID:            id,
		Pattern:       pattern,
		FileType:      fileAccess,
		FileAuthority: fileAuthority,
		PreventWrite:  true,
		PreventRead:   preventRead,
		CreatedBy:     createdBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	_, err = db.Exec(
		`INSERT INTO file_rules (id, pattern, file_access, file_authority, prevent_write, prevent_read, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.Pattern, f.FileType, f.FileAuthority, f.PreventWrite, f.PreventRead, f.CreatedBy, f.CreatedAt, f.UpdatedAt,
	)
	if err != nil {
		if isDuplicatePatternError(err) {
			return nil, fmt.Errorf("store: add file rule: %w: %s", ErrDuplicatePattern, pattern)
		}
		return nil, fmt.Errorf("store: add file rule: %w", err)
	}
	return &f, nil
}

// ListFileRules returns all file rules ordered by creation time.
func ListFileRules(db *sql.DB) ([]FileRule, error) {
	rows, err := db.Query(
		`SELECT id, pattern, file_access, file_authority, prevent_write, prevent_read, created_by, created_at, updated_at
		 FROM file_rules ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list file rules: %w", err)
	}
	defer rows.Close()

	var rules []FileRule
	for rows.Next() {
		var f FileRule
		var pw, pr int
		if err := rows.Scan(&f.ID, &f.Pattern, &f.FileType, &f.FileAuthority, &pw, &pr, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan file rule: %w", err)
		}
		f.PreventWrite = pw != 0
		f.PreventRead = pr != 0
		rules = append(rules, f)
	}
	return rules, rows.Err()
}

// RemoveFileRule deletes the file rule with the given pattern.
// Returns (true, nil) if a rule was removed, (false, nil) if no matching rule exists.
func RemoveFileRule(db *sql.DB, pattern string) (bool, error) {
	res, err := db.Exec(`DELETE FROM file_rules WHERE pattern = ?`, pattern)
	if err != nil {
		return false, fmt.Errorf("store: remove file rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: remove file rule rows affected: %w", err)
	}
	return n > 0, nil
}

// FileRuleForPath returns the effective deny file rule whose pattern covers
// filePath, or nil if the file is not protected.
//
// Allow rules always supersede deny rules: if any matching rule has FileType
// "allow", the file is considered unprotected and nil is returned. If only
// deny rules match, the first matching deny rule is returned.
//
// repoRoot is the absolute repo root path. It is used to convert absolute
// filePaths to repo-relative paths before matching, so that relative patterns
// like "*.gitignore" or "src/" match absolute paths received from the hook.
// Pass an empty string to skip relative-path matching.
func FileRuleForPath(db *sql.DB, filePath, repoRoot string) (*FileRule, error) {
	rules, err := ListFileRules(db)
	if err != nil {
		return nil, err
	}

	var firstDeny *FileRule
	for _, f := range rules {
		if !pathMatchesFileRule(f.Pattern, filePath, repoRoot) {
			continue
		}
		if f.FileType == "allow" {
			return nil, nil // allow supersedes all deny rules
		}
		if firstDeny == nil {
			fCopy := f
			firstDeny = &fCopy
		}
	}
	return firstDeny, nil
}

// NormalizePattern converts an absolute path pattern to a repo-relative path.
// Glob patterns and already-relative patterns are returned unchanged.
// If the absolute path is outside the repo root, it is returned as-is.
func NormalizePattern(pattern, repoRoot string) string {
	if !filepath.IsAbs(pattern) {
		return pattern
	}
	if repoRoot == "" {
		return pattern
	}
	rel, err := filepath.Rel(repoRoot, pattern)
	if err != nil || strings.HasPrefix(rel, "..") {
		return pattern // outside repo — keep absolute
	}
	return rel
}

// StandardGuardrailFileRules is the default set of guardrail file rules offered
// during `cordon init`. All are seeded with prevent_read=true so agents cannot
// read credential files into their context. They are stored as normal file rules
// in policy.db, so they appear in `cordon file list` and can be removed if desired.
var StandardGuardrailFileRules = []struct {
	Pattern     string
	PreventRead bool
}{
	// Environment / secrets files
	{".env", true},
	{".env.*", true},
	{".envrc", true},
	// Cloud / service credentials
	{"credentials.json", true},
	{"secrets.json", true},
	{"service-account.json", true},
	// Certificates and private keys
	{"*.pem", true},
	{"*.key", true},
	{"*.p12", true},
	{"*.pfx", true},
}

// newUUID generates a random UUID v4 string without external dependencies.
func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
