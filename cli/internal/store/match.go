package store

import (
	"path/filepath"
	"strings"
)

// pathMatchesFileRule reports whether filePath is covered by the given file rule pattern.
//
// repoRoot is the absolute path to the repository root. When provided, the
// absolute filePath is first converted to a repo-relative path and matching
// is tried against both the absolute and relative forms. This allows patterns
// stored as relative paths (e.g. ".gitignore", "src/", "*.go") to match
// absolute file paths received from the hook payload.
//
// Three matching strategies are tried against each path form:
//  1. Exact match after path cleaning.
//  2. Single-level glob via filepath.Match (e.g. "src/*.go", "*.gitignore").
//  3. Directory prefix match: filePath is somewhere inside the rule directory.
//
// Note: double-star (**) globs are not yet supported.
func pathMatchesFileRule(pattern, filePath, repoRoot string) bool {
	if matchOnePath(pattern, filePath) {
		return true
	}
	// Also try matching against the repo-relative path.
	if repoRoot != "" {
		if rel, err := filepath.Rel(repoRoot, filePath); err == nil && !strings.HasPrefix(rel, "..") {
			if matchOnePath(pattern, rel) {
				return true
			}
		}
	}
	return false
}

// matchOnePath tries all three matching strategies for a single (pattern, filePath) pair.
func matchOnePath(pattern, filePath string) bool {
	cleanPattern := filepath.Clean(pattern)
	cleanFile := filepath.Clean(filePath)

	// 1. Exact match.
	if cleanPattern == cleanFile {
		return true
	}

	// 2. Single-level glob match.
	if matched, err := filepath.Match(cleanPattern, cleanFile); err == nil && matched {
		return true
	}

	// 3. Directory prefix: filePath is inside the rule directory.
	// We add the OS separator to both to avoid false positives where the
	// pattern is a prefix of the file name (e.g. "src" should not match "src2/main.go").
	prefix := cleanPattern + string(filepath.Separator)
	if strings.HasPrefix(cleanFile+string(filepath.Separator), prefix) {
		return true
	}

	return false
}
