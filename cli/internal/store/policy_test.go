package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestFileRuleForPath_ExactMatch(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddFileRule(db, ".env", "deny", "standard", "test", false); err != nil {
		t.Fatal(err)
	}

	rule, err := FileRuleForPath(db, ".env", "")
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected rule to match, got nil")
	}
	if rule.Pattern != ".env" {
		t.Errorf("pattern = %q, want .env", rule.Pattern)
	}
}

func TestFileRuleForPath_GlobMatch(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddFileRule(db, "*.pem", "deny", "standard", "test", false); err != nil {
		t.Fatal(err)
	}

	rule, err := FileRuleForPath(db, "server.pem", "")
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected glob to match server.pem, got nil")
	}
}

func TestFileRuleForPath_DirectoryPrefix(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddFileRule(db, "src", "deny", "standard", "test", false); err != nil {
		t.Fatal(err)
	}

	rule, err := FileRuleForPath(db, "src/main.go", "")
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected directory prefix to match src/main.go, got nil")
	}
}

func TestFileRuleForPath_AllowOverridesDeny(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddFileRule(db, "src", "deny", "standard", "test", false); err != nil {
		t.Fatal(err)
	}
	if _, err := AddFileRule(db, "src/main.go", "allow", "standard", "test", false); err != nil {
		t.Fatal(err)
	}

	rule, err := FileRuleForPath(db, "src/main.go", "")
	if err != nil {
		t.Fatal(err)
	}
	if rule != nil {
		t.Errorf("expected allow to override deny, got rule %q", rule.Pattern)
	}
}

func TestFileRuleForPath_NoMatch(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddFileRule(db, ".env", "deny", "standard", "test", false); err != nil {
		t.Fatal(err)
	}

	rule, err := FileRuleForPath(db, "README.md", "")
	if err != nil {
		t.Fatal(err)
	}
	if rule != nil {
		t.Errorf("expected no match, got rule %q", rule.Pattern)
	}
}

func TestFileRuleForPath_AbsolutePathMatchesRelativeRule(t *testing.T) {
	db := newTestPolicyDB(t)
	repoRoot := filepath.Join(string(filepath.Separator), "tmp", "repo")
	absPath := filepath.Join(repoRoot, ".env")
	if _, err := AddFileRule(db, ".env", "deny", "standard", "test", false); err != nil {
		t.Fatal(err)
	}

	rule, err := FileRuleForPath(db, absPath, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected relative rule to match absolute path inside repo, got nil")
	}
}

func TestFileRuleForPath_DirectoryPrefixNoFalsePositive(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddFileRule(db, "src", "deny", "standard", "test", false); err != nil {
		t.Fatal(err)
	}

	rule, err := FileRuleForPath(db, "src2/main.go", "")
	if err != nil {
		t.Fatal(err)
	}
	if rule != nil {
		t.Fatalf("expected no match for sibling prefix path, got %q", rule.Pattern)
	}
}

func TestAddFileRule_DuplicatePattern(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddFileRule(db, ".env", "deny", "standard", "test", false); err != nil {
		t.Fatal(err)
	}
	_, err := AddFileRule(db, ".env", "deny", "standard", "test", false)
	if !errors.Is(err, ErrDuplicatePattern) {
		t.Errorf("expected ErrDuplicatePattern, got %v", err)
	}
}

func TestNormalizeFilePath_AbsoluteInsideRepo(t *testing.T) {
	repo := filepath.Join(string(filepath.Separator), "tmp", "repo")
	in := filepath.Join(repo, "src", "main.go")
	got := NormalizeFilePath(in, repo)
	if got != filepath.Join("src", "main.go") {
		t.Fatalf("NormalizeFilePath(%q, %q) = %q, want %q", in, repo, got, filepath.Join("src", "main.go"))
	}
}

func TestNormalizeFilePath_AbsoluteOutsideRepo(t *testing.T) {
	repo := filepath.Join(string(filepath.Separator), "tmp", "repo")
	in := filepath.Join(string(filepath.Separator), "tmp", "other", "main.go")
	got := NormalizeFilePath(in, repo)
	if got != in {
		t.Fatalf("NormalizeFilePath(%q, %q) = %q, want %q", in, repo, got, in)
	}
}

func TestNormalizeFilePath_RelativeCleaned(t *testing.T) {
	in := filepath.Join(".", "src", "..", "src", "main.go")
	got := NormalizeFilePath(in, filepath.Join(string(filepath.Separator), "tmp", "repo"))
	if got != filepath.Join("src", "main.go") {
		t.Fatalf("NormalizeFilePath(%q, repo) = %q, want %q", in, got, filepath.Join("src", "main.go"))
	}
}

func TestNormalizePattern_Table(t *testing.T) {
	repo := filepath.Join(string(filepath.Separator), "tmp", "repo")
	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{
			name:    "relative unchanged",
			pattern: filepath.Join("src", "main.go"),
			want:    filepath.Join("src", "main.go"),
		},
		{
			name:    "absolute inside repo relativized",
			pattern: filepath.Join(repo, "src", "main.go"),
			want:    filepath.Join("src", "main.go"),
		},
		{
			name:    "absolute outside repo unchanged",
			pattern: filepath.Join(string(filepath.Separator), "tmp", "other", "main.go"),
			want:    filepath.Join(string(filepath.Separator), "tmp", "other", "main.go"),
		},
		{
			name:    "glob unchanged",
			pattern: "*.env",
			want:    "*.env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizePattern(tt.pattern, repo)
			if got != tt.want {
				t.Fatalf("NormalizePattern(%q, %q) = %q, want %q", tt.pattern, repo, got, tt.want)
			}
		})
	}
}
