package tests

import "testing"

func TestInitYesAddsDefaultGuardrails(t *testing.T) {
	repo := testRepo{
		Dir:  t.TempDir(),
		Home: t.TempDir(),
	}

	runCordon(t, repo, "init", "-y")

	var commandList struct {
		Rules []struct {
			Pattern string `json:"Pattern"`
		} `json:"rules"`
	}
	r := runCordon(t, repo, "command", "list", "--json")
	mustParseJSON(t, r.Stdout, &commandList)

	if len(commandList.Rules) == 0 {
		t.Fatal("expected default command guardrails after init -y, got none")
	}

	foundCommandGuardrail := false
	for _, rule := range commandList.Rules {
		if rule.Pattern == "git reset --hard*" {
			foundCommandGuardrail = true
			break
		}
	}
	if !foundCommandGuardrail {
		t.Fatal("expected guardrail 'git reset --hard*' after init -y")
	}

	var fileList struct {
		FileRules []struct {
			Pattern string `json:"Pattern"`
		} `json:"file_rules"`
	}
	r = runCordon(t, repo, "file", "list", "--json")
	mustParseJSON(t, r.Stdout, &fileList)

	if len(fileList.FileRules) == 0 {
		t.Fatal("expected default file guardrails after init -y, got none")
	}

	foundFileGuardrail := false
	for _, rule := range fileList.FileRules {
		if rule.Pattern == ".env" {
			foundFileGuardrail = true
			break
		}
	}
	if !foundFileGuardrail {
		t.Fatal("expected file guardrail '.env' after init -y")
	}
}
