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
