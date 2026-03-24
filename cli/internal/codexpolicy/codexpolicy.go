// Package codexpolicy generates the Codex model instructions file at
// .cordon/codex-policy.md. This file is read by Codex on each turn and
// instructs it not to write to any protected file paths.
//
// This is soft enforcement: Codex follows the instructions reliably but the
// file can theoretically be ignored. The notify hook (agent-turn-complete)
// is used to detect violations after each turn.
package codexpolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cordon-co/cordon-cli/cli/internal/store"
)

const filename = "codex-policy.md"

// Generate writes .cordon/codex-policy.md for the given repo root using the
// provided file rule list. If rules is empty, the file is written with an
// empty deny list (no restrictions).
//
// The file is replaced atomically (write to temp, rename) to avoid partial
// reads by Codex during a live session.
func Generate(repoRoot string, rules []store.FileRule) error {
	content := buildContent(rules)

	dir := filepath.Join(repoRoot, ".cordon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("codexpolicy: create .cordon directory: %w", err)
	}

	dest := filepath.Join(dir, filename)
	tmp := dest + ".tmp"

	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("codexpolicy: write temp file: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("codexpolicy: rename to %s: %w", dest, err)
	}
	return nil
}

func buildContent(rules []store.FileRule) string {
	var b strings.Builder

	b.WriteString("# Cordon Policy — Do Not Modify\n\n")
	b.WriteString("This file is managed by Cordon and regenerated automatically when file rules change.\n\n")

	if len(rules) == 0 {
		b.WriteString("No file rules are currently configured. All file writes are permitted.\n")
		return b.String()
	}

	b.WriteString("## Protected Files\n\n")
	b.WriteString("You MUST NOT write to any of the following files, folders, or patterns ")
	b.WriteString("unless the user has explicitly issued you a Cordon pass.\n\n")
	b.WriteString("If you need to modify a protected file, use the `cordon_request_access` MCP tool ")
	b.WriteString("to request a pass, or ask the user to run `cordon pass issue --file <path>`.\n\n")
	b.WriteString("This is an enforced policy. Do not attempt to write to protected paths via any ")
	b.WriteString("alternative method, including shell commands such as echo, sed, tee, cp, or mv.\n\n")
	b.WriteString("### Deny List\n\n")

	for _, f := range rules {
		if f.FileType == "allow" {
			continue // allow rules permit access; omit from deny list
		}
		label := ""
		if f.FileAuthority == "elevated" {
			label = " *(elevated rule — requires elevated/admin pass)*"
		}
		fmt.Fprintf(&b, "- `%s`%s\n", f.Pattern, label)
	}

	return b.String()
}
