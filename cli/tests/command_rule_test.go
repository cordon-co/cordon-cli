package tests

import (
	"testing"
)

func TestCommandRuleLifecycle(t *testing.T) {
	repo := initRepo(t)

	// Add a deny command rule.
	var addResult struct {
		Rule struct {
			Pattern  string `json:"Pattern"`
			RuleType string `json:"RuleType"`
		} `json:"rule"`
	}
	r := runCordon(t, repo, "command", "add", "rm -rf *", "--json")
	mustParseJSON(t, r.Stdout, &addResult)
	if addResult.Rule.Pattern != "rm -rf *" {
		t.Errorf("add: pattern = %q, want 'rm -rf *'", addResult.Rule.Pattern)
	}
	if addResult.Rule.RuleType != "deny" {
		t.Errorf("add: rule_type = %q, want deny", addResult.Rule.RuleType)
	}

	// List should contain the rule.
	var listResult struct {
		Rules []struct {
			Pattern string `json:"Pattern"`
		} `json:"rules"`
	}
	r = runCordon(t, repo, "command", "list", "--json")
	mustParseJSON(t, r.Stdout, &listResult)
	found := false
	for _, rule := range listResult.Rules {
		if rule.Pattern == "rm -rf *" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("list: rule 'rm -rf *' not found in %+v", listResult.Rules)
	}

	// Remove the rule.
	runCordon(t, repo, "command", "remove", "rm -rf *")

	// List should no longer contain the rule.
	r = runCordon(t, repo, "command", "list", "--json")
	mustParseJSON(t, r.Stdout, &listResult)
	for _, rule := range listResult.Rules {
		if rule.Pattern == "rm -rf *" {
			t.Error("remove: rule 'rm -rf *' still present after removal")
		}
	}
}
