package tests

import (
	"testing"
)

func TestFileRuleLifecycle(t *testing.T) {
	repo := initRepo(t)

	// Add a file rule.
	var addResult struct {
		FileRule struct {
			Pattern  string `json:"Pattern"`
			FileType string `json:"FileType"`
		} `json:"file_rule"`
	}
	r := runCordon(t, repo, "file", "add", "src/*.go", "--json")
	mustParseJSON(t, r.Stdout, &addResult)
	if addResult.FileRule.Pattern != "src/*.go" {
		t.Errorf("add: pattern = %q, want src/*.go", addResult.FileRule.Pattern)
	}
	if addResult.FileRule.FileType != "deny" {
		t.Errorf("add: file_type = %q, want deny", addResult.FileRule.FileType)
	}

	// List should contain the rule.
	var listResult struct {
		FileRules []struct {
			Pattern string `json:"Pattern"`
		} `json:"file_rules"`
	}
	r = runCordon(t, repo, "file", "list", "--json")
	mustParseJSON(t, r.Stdout, &listResult)
	found := false
	for _, fr := range listResult.FileRules {
		if fr.Pattern == "src/*.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("list: rule src/*.go not found in %+v", listResult.FileRules)
	}

	// Remove the rule.
	runCordon(t, repo, "file", "remove", "src/*.go")

	// List should no longer contain the rule.
	r = runCordon(t, repo, "file", "list", "--json")
	mustParseJSON(t, r.Stdout, &listResult)
	for _, fr := range listResult.FileRules {
		if fr.Pattern == "src/*.go" {
			t.Error("remove: rule src/*.go still present after removal")
		}
	}
}
