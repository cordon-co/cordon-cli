package store

import (
	"path"
	"path/filepath"
	"regexp"
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
//  2. Glob match (supports *, ?, [], and ** for recursive path matching).
//  3. Directory prefix match: filePath is somewhere inside the rule directory.
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

	// 2. Glob match.
	if matchPathPattern(cleanPattern, cleanFile) {
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

func matchPathPattern(pattern, filePath string) bool {
	p := filepath.ToSlash(pattern)
	f := filepath.ToSlash(filePath)

	if strings.Contains(p, "**") {
		return matchDoubleStarPattern(p, f)
	}
	matched, err := path.Match(p, f)
	return err == nil && matched
}

func matchDoubleStarPattern(pattern, filePath string) bool {
	re, err := regexp.Compile(doubleStarToRegexp(pattern))
	if err != nil {
		return false
	}
	return re.MatchString(filePath)
}

func doubleStarToRegexp(pattern string) string {
	var b strings.Builder
	b.WriteString("^")

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString(`[^/]*`)
			}
		case '?':
			b.WriteString(`[^/]`)
		case '[':
			// Preserve simple character class semantics.
			j := i + 1
			for j < len(pattern) && pattern[j] != ']' {
				j++
			}
			if j < len(pattern) {
				b.WriteByte('[')
				b.WriteString(pattern[i+1 : j])
				b.WriteByte(']')
				i = j
			} else {
				b.WriteString(`\[`)
			}
		default:
			if strings.ContainsRune(`.+()|{}^$\\`, rune(ch)) {
				b.WriteByte('\\')
			}
			b.WriteByte(ch)
		}
	}

	b.WriteString("$")
	return b.String()
}
