package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cordon-co/cordon-cli/cli/internal/agents"
)

func TestInitYesAutoSelectsAllInstallableAgents(t *testing.T) {
	repo := testRepo{
		Dir:  t.TempDir(),
		Home: t.TempDir(),
	}

	var out struct {
		Agents []string `json:"agents"`
	}
	r := runCordon(t, repo, "init", "-y", "--json")
	mustParseJSON(t, r.Stdout, &out)

	want := 0
	for _, a := range agents.All() {
		if a.Installable() {
			want++
		}
	}
	if len(out.Agents) != want {
		t.Fatalf("init -y selected %d agents, want %d", len(out.Agents), want)
	}
}

func TestInitAgentFlagReinstallsAgentSupportInInitialisedRepo(t *testing.T) {
	repo := testRepo{
		Dir:  t.TempDir(),
		Home: t.TempDir(),
	}

	runCordon(t, repo, "init", "-y")

	codexHooksPath := filepath.Join(repo.Dir, ".codex", "hooks.json")
	if err := os.Remove(codexHooksPath); err != nil {
		t.Fatalf("remove codex hooks: %v", err)
	}

	runCordon(t, repo, "init", "--agent", "-y")

	if _, err := os.Stat(codexHooksPath); err != nil {
		t.Fatalf("expected codex hooks to be recreated by init --agent -y: %v", err)
	}
}
