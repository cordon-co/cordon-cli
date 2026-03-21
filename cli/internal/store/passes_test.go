package store

import (
	"testing"
	"time"
)

func TestActivePassForPath_FileScopedMatch(t *testing.T) {
	policyDB := newTestPolicyDB(t)
	dataDB := newTestDataDB(t)

	rule, err := AddFileRule(policyDB, "src/config.go", "deny", "standard", "test", false)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	p := Pass{
		FileRuleID: rule.ID,
		Pattern:    rule.Pattern,
		FilePath:   "src/config.go",
		IssuedTo:   "test",
		IssuedBy:   "test",
		Status:     "active",
		IssuedAt:   now.Format(time.RFC3339),
		ExpiresAt:  now.Add(time.Hour).Format(time.RFC3339),
	}
	if err := IssuePass(dataDB, p); err != nil {
		t.Fatal(err)
	}

	active, err := ActivePassForPath(dataDB, "src/config.go", "")
	if err != nil {
		t.Fatal(err)
	}
	if active == nil {
		t.Fatal("expected active pass, got nil")
	}
}

func TestActivePassForPath_RuleWideMatch(t *testing.T) {
	policyDB := newTestPolicyDB(t)
	dataDB := newTestDataDB(t)

	rule, err := AddFileRule(policyDB, "src", "deny", "standard", "test", false)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	// Rule-wide pass: FilePath is empty, Pattern covers the directory.
	p := Pass{
		FileRuleID: rule.ID,
		Pattern:    rule.Pattern,
		FilePath:   "", // rule-wide
		IssuedTo:   "test",
		IssuedBy:   "test",
		Status:     "active",
		IssuedAt:   now.Format(time.RFC3339),
		ExpiresAt:  now.Add(time.Hour).Format(time.RFC3339),
	}
	if err := IssuePass(dataDB, p); err != nil {
		t.Fatal(err)
	}

	active, err := ActivePassForPath(dataDB, "src/main.go", "")
	if err != nil {
		t.Fatal(err)
	}
	if active == nil {
		t.Fatal("expected rule-wide pass to cover src/main.go, got nil")
	}
}

func TestActivePassForPath_ExpiredNoMatch(t *testing.T) {
	policyDB := newTestPolicyDB(t)
	dataDB := newTestDataDB(t)

	rule, err := AddFileRule(policyDB, "src/config.go", "deny", "standard", "test", false)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	p := Pass{
		FileRuleID: rule.ID,
		Pattern:    rule.Pattern,
		FilePath:   "src/config.go",
		IssuedTo:   "test",
		IssuedBy:   "test",
		Status:     "active",
		IssuedAt:   now.Add(-2 * time.Hour).Format(time.RFC3339),
		ExpiresAt:  now.Add(-time.Hour).Format(time.RFC3339), // already expired
	}
	if err := IssuePass(dataDB, p); err != nil {
		t.Fatal(err)
	}

	active, err := ActivePassForPath(dataDB, "src/config.go", "")
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Errorf("expected expired pass to return nil, got pass %s", active.ID)
	}
}
