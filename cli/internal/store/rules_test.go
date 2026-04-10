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

func TestMatchCommandRule_ArgvFlagMatchOutOfOrder(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "git push --force*", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "git push -u origin main --force")
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected argv option match for reordered --force flag, got nil")
	}
}

func TestMatchCommandRule_ArgvFlagRequiresCommandPrefix(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "git push --force*", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "git pull --force")
	if err != nil {
		t.Fatal(err)
	}
	if rule != nil {
		t.Fatalf("expected no match for non-push command, got %q", rule.Pattern)
	}
}

func TestMatchCommandRule_ArgvShortFlagMatchOutOfOrder(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "git push -f*", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "git push origin main -f")
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected argv option match for reordered -f flag, got nil")
	}
}

func TestMatchCommandRule_ArgvShortFlagRequiresCommandPrefix(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "git push -f*", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "git pull -f")
	if err != nil {
		t.Fatal(err)
	}
	if rule != nil {
		t.Fatalf("expected no match for non-push command, got %q", rule.Pattern)
	}
}

func TestMatchCommandRule_PrefixMatchForPlainPattern(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "echo", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "echo hello")
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected plain pattern to match command prefix, got nil")
	}
}

func TestMatchCommandRule_PlainPatternDoesNotMatchDifferentCommand(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "echo", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "echos hello")
	if err != nil {
		t.Fatal(err)
	}
	if rule != nil {
		t.Fatalf("expected no match for different command, got %q", rule.Pattern)
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

func TestMatchCommandRule_ArgvDoesNotReorderWhenPositionalSuffixPresent(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "git push --force origin", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "git push origin --force")
	if err != nil {
		t.Fatal(err)
	}
	if rule != nil {
		t.Fatalf("expected no match when positional suffix exists after option in pattern, got %q", rule.Pattern)
	}
}

func TestMatchCommandRule_ParseFailureFallsBackToStringMatch(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "echo *", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	// Unterminated quote is invalid shell; argv matcher should fail closed, while
	// string matcher still covers this pattern.
	rule, err := MatchCommandRule(db, `echo "unterminated`)
	if err != nil {
		t.Fatal(err)
	}
	if rule == nil {
		t.Fatal("expected string matcher fallback to match parse-failed command")
	}
}

func TestMatchCommandRule_CommandAndCommandStarEquivalent(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "git commit *", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	tests := []string{
		"git commit",
		"git commit -m test",
	}
	for _, cmd := range tests {
		t.Run(cmd, func(t *testing.T) {
			rule, err := MatchCommandRule(db, cmd)
			if err != nil {
				t.Fatal(err)
			}
			if rule == nil {
				t.Fatalf("expected %q to match git commit * rule", cmd)
			}
		})
	}
}

func TestMatchCommandRule_AllowOverrideForCommandStar(t *testing.T) {
	db := newTestPolicyDB(t)
	if _, err := AddRule(db, "git commit", "deny", "standard", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddRule(db, "git commit *", "allow", "standard", "test"); err != nil {
		t.Fatal(err)
	}

	rule, err := MatchCommandRule(db, "git commit -m test")
	if err != nil {
		t.Fatal(err)
	}
	if rule != nil {
		t.Fatalf("expected allow rule git commit * to override deny, got deny %q", rule.Pattern)
	}
}
