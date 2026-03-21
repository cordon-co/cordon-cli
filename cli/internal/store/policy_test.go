package store

import (
	"errors"
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
