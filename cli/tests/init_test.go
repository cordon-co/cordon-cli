package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestInitRecreatesUserDataWhenRepoAlreadyInitialised(t *testing.T) {
	repo := testRepo{
		Dir:  t.TempDir(),
		Home: t.TempDir(),
	}

	var first struct {
		DataDB string `json:"data_db"`
	}
	r := runCordon(t, repo, "init", "-y", "--json")
	mustParseJSON(t, r.Stdout, &first)
	if first.DataDB == "" {
		t.Fatal("expected initial init to return data_db path")
	}

	if err := os.RemoveAll(filepath.Join(repo.Home, ".cordon")); err != nil {
		t.Fatalf("remove ~/.cordon: %v", err)
	}

	var second struct {
		AlreadyInitialised bool   `json:"already_initialised"`
		DataDB             string `json:"data_db"`
	}
	r = runCordon(t, repo, "init", "--json")
	mustParseJSON(t, r.Stdout, &second)

	if !second.AlreadyInitialised {
		t.Fatal("expected already_initialised=true for existing .cordon setup")
	}
	if second.DataDB == "" {
		t.Fatal("expected init --json to return data_db path for existing .cordon setup")
	}
	if _, err := os.Stat(second.DataDB); err != nil {
		t.Fatalf("expected data db to be recreated: %v", err)
	}
}

func TestInitMentionsExistingConfiguredCordon(t *testing.T) {
	repo := testRepo{
		Dir:  t.TempDir(),
		Home: t.TempDir(),
	}

	runCordon(t, repo, "init", "-y")
	r := runCordon(t, repo, "init")

	stdout := string(r.Stdout)
	if !strings.Contains(stdout, ".cordon/ found and already configured for this repository") {
		t.Fatalf("expected existing-setup note in output, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "User data ready at") {
		t.Fatalf("expected user-data note in output, got:\n%s", stdout)
	}
}
