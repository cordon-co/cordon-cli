package tests

import (
	"testing"
)

func TestPassLifecycle(t *testing.T) {
	repo := initRepo(t)

	// A pass requires a file rule to exist first.
	runCordon(t, repo, "file", "add", ".env")

	// Issue a pass.
	var issueResult struct {
		Pass struct {
			ID     string `json:"ID"`
			Status string `json:"Status"`
		} `json:"pass"`
	}
	r := runCordon(t, repo, "pass", "issue", ".env", "--duration", "60m", "--json")
	mustParseJSON(t, r.Stdout, &issueResult)
	if issueResult.Pass.ID == "" {
		t.Fatal("issue: pass ID is empty")
	}
	if issueResult.Pass.Status != "active" {
		t.Errorf("issue: status = %q, want active", issueResult.Pass.Status)
	}

	passID := issueResult.Pass.ID

	// List should show the active pass.
	var listResult struct {
		Passes []struct {
			ID     string `json:"ID"`
			Status string `json:"Status"`
		} `json:"passes"`
	}
	r = runCordon(t, repo, "pass", "list", "--json")
	mustParseJSON(t, r.Stdout, &listResult)
	found := false
	for _, p := range listResult.Passes {
		if p.ID == passID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("list: pass %s not found in active passes", passID)
	}

	// Revoke the pass.
	var revokeResult struct {
		Revoked bool `json:"revoked"`
	}
	r = runCordon(t, repo, "pass", "revoke", passID, "--json")
	mustParseJSON(t, r.Stdout, &revokeResult)
	if !revokeResult.Revoked {
		t.Errorf("revoke: expected revoked=true, got false")
	}

	// Active list should now be empty.
	r = runCordon(t, repo, "pass", "list", "--json")
	mustParseJSON(t, r.Stdout, &listResult)
	for _, p := range listResult.Passes {
		if p.ID == passID {
			t.Errorf("revoke: pass %s still in active list after revoke", passID)
		}
	}
}

func TestPassRequiresFileRule(t *testing.T) {
	repo := initRepo(t)

	// Issuing a pass for a file with no rule should fail.
	r := runCordonRaw(t, repo, "pass", "issue", "no-rule-here.txt", "--json")
	if r.ExitCode == 0 {
		t.Error("expected non-zero exit when issuing pass for uncovered file, got 0")
	}
}

func TestPassIssueForCommandRule(t *testing.T) {
	repo := initRepo(t)

	runCordon(t, repo, "command", "add", "git push --force*")

	var issueResult struct {
		Pass struct {
			ID       string `json:"ID"`
			Pattern  string `json:"Pattern"`
			FilePath string `json:"FilePath"`
			Status   string `json:"Status"`
		} `json:"pass"`
	}
	r := runCordon(t, repo, "pass", "issue", "git push --force origin main", "--duration", "60m", "--json")
	mustParseJSON(t, r.Stdout, &issueResult)
	if issueResult.Pass.ID == "" {
		t.Fatal("issue command pass: pass ID is empty")
	}
	if issueResult.Pass.Status != "active" {
		t.Fatalf("issue command pass: status = %q, want active", issueResult.Pass.Status)
	}
	if issueResult.Pass.Pattern != "git push --force*" {
		t.Fatalf("issue command pass: pattern = %q, want git push --force*", issueResult.Pass.Pattern)
	}
	if issueResult.Pass.FilePath != "" {
		t.Fatalf("issue command pass: file_path = %q, want empty", issueResult.Pass.FilePath)
	}
}
