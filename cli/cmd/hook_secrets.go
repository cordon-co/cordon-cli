package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cordon-co/cordon-cli/cli/internal/api"
	"github.com/cordon-co/cordon-cli/cli/internal/hook"
	"github.com/cordon-co/cordon-cli/cli/internal/secrets"
)

type secretScanner interface {
	ScanAndRedact(toolInput json.RawMessage) (secrets.ScanResult, error)
}

func applySecretDetection(event *hook.Event, hookErr error, outW io.Writer, errW io.Writer, scanner secretScanner, action string) error {
	if event == nil || scanner == nil {
		return hookErr
	}
	// "allow" disables secret censoring and never adds a secret-based deny.
	if action == api.SecretDetectionActionAllow {
		return hookErr
	}

	scan, err := scanner.ScanAndRedact(event.ToolInput)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cordon: secret detection failed: %v\n", err)
		return hookErr
	}
	event.ToolInput = scan.RedactedToolInput
	event.CommandRaw = applySecretRedactions(event.CommandRaw, scan.Redactions)
	event.CommandOpsJSON = applySecretRedactions(event.CommandOpsJSON, scan.Redactions)
	event.SecretRuleIDs = scan.RuleIDs
	event.SecretsDetected = len(scan.RuleIDs) > 0

	if hookErr == hook.ErrDenied {
		return hookErr
	}
	if !event.SecretsDetected || action != api.SecretDetectionActionDeny {
		return hookErr
	}

	reason := secretDenyReason(scan.RuleIDs)
	if err := hook.WriteCustomDeny(outW, errW, event.ToolName, reason); err != nil {
		return err
	}
	event.Decision = hook.DecisionDeny
	event.DeniedOpReason = "secret detection policy violation"
	if event.DeniedOpIndex == 0 {
		event.DeniedOpIndex = -1
	}
	return hook.ErrDenied
}

func secretDenyReason(ruleIDs []string) string {
	if len(ruleIDs) == 0 {
		return "CORDON POLICY: detected secret content in tool input."
	}
	return "CORDON POLICY: detected secret content (rules: " + strings.Join(ruleIDs, ", ") + "). Secret detection action is set to deny in ~/.cordon/config.json."
}

func applySecretRedactions(input string, redactions []secrets.Redaction) string {
	out := input
	for _, r := range redactions {
		if r.Secret == "" || r.RuleID == "" {
			continue
		}
		out = strings.ReplaceAll(out, r.Secret, "<SECRET:"+r.RuleID+">")
	}
	return out
}
