package store

import (
	"testing"
)

func TestMatchCommandRule_ExactMatch(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "cat .env", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "cat .env")
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected rule to match, got nil")
	}
}

func TestMatchCommandRule_GlobMatch(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "git push --force*", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "git push --force-with-lease origin main")
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected glob to match git push --force-with-lease, got nil")
	}
}

func TestMatchCommandRule_AllowOverridesDeny(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "git push --force*", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddRule(db, "git push --force-with-lease origin main", "allow", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "git push --force-with-lease origin main")
	if err != nil {
		t.Fatal(err)
	}
	if rule != nil {
		t.Errorf("expected allow to override deny, got rule %q", rule.Pattern)
	}
}

func TestMatchCommandRule_NoMatch(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "rm -rf /*", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "go build ./...")
	if err != nil {
		t.Fatal(err)
	}
	if rule != nil {
		t.Errorf("expected no match, got rule %q", rule.Pattern)
	}
}
