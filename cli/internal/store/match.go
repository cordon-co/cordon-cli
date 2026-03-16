package store

import (
	"path/filepath"
	"strings"
)

// pathMatchesZone reports whether filePath is covered by the given zone pattern.
// Three matching strategies are tried in order:
//  1. Exact match after path cleaning.
//  2. Single-level glob via filepath.Match (e.g. "src/*.go").
//  3. Directory prefix match: filePath is somewhere inside the zone directory.
//
// Note: double-star (**) globs are not yet supported. Patterns containing **
// will only match if filepath.Match happens to produce a result, which it will
// not for cross-directory patterns.
func pathMatchesZone(pattern, filePath string) bool {
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

	// 3. Directory prefix: filePath is inside the zone directory.
	// We add the OS separator to both to avoid false positives where the
	// pattern is a prefix of the file name (e.g. "src" should not match "src2/main.go").
	prefix := cleanPattern + string(filepath.Separator)
	if strings.HasPrefix(cleanFile+string(filepath.Separator), prefix) {
		return true
	}

	return false
}
