package agents

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cordon-co/cordon/internal/codexpolicy"
	"github.com/cordon-co/cordon/internal/store"
)

// Codex configures OpenAI Codex via .cordon/codex-policy.md (soft enforcement
// through model instructions).
type Codex struct{}

func (c *Codex) ID() string          { return "codex" }
func (c *Codex) DisplayName() string { return "Codex" }
func (c *Codex) Installable() bool   { return true }

func (c *Codex) Install(repoRoot string) error {
	// Generate codex-policy.md from current file rules (may be empty on first init).
	rules, err := c.loadRules(repoRoot)
	if err != nil {
		// If we can't load rules (e.g. DB not ready yet), generate with empty list.
		rules = nil
	}
	return codexpolicy.Generate(repoRoot, rules)
}

func (c *Codex) Remove(repoRoot string) error {
	path := filepath.Join(repoRoot, ".cordon", "codex-policy.md")
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (c *Codex) Installed(repoRoot string) bool {
	path := filepath.Join(repoRoot, ".cordon", "codex-policy.md")
	_, err := os.Stat(path)
	return err == nil
}

// loadRules reads the current file rules from the policy database.
func (c *Codex) loadRules(repoRoot string) ([]store.FileRule, error) {
	policyDB, err := store.OpenPolicyDB(repoRoot)
	if err != nil {
		return nil, err
	}
	defer policyDB.Close()
	return store.ListFileRules(policyDB)
}
