package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cordon-co/cordon-cli/cli/internal/api"
	"github.com/cordon-co/cordon-cli/cli/internal/hook"
	"github.com/cordon-co/cordon-cli/cli/internal/secrets"
)

type fakeSecretScanner struct {
	result secrets.ScanResult
	err    error
}

func (f fakeSecretScanner) ScanAndRedact(_ json.RawMessage) (secrets.ScanResult, error) {
	return f.result, f.err
}

func TestApplySecretDetection_DefaultCensorAllowsAndRedacts(t *testing.T) {
	e := &hook.Event{ToolName: "Write", ToolInput: json.RawMessage(`{"content":"ghp_foo"}`), Decision: hook.DecisionAllow}
	scanner := fakeSecretScanner{result: secrets.ScanResult{
		RedactedToolInput: json.RawMessage(`{"content":"<SECRET:github-pat>"}`),
		RuleIDs:           []string{"github-pat"},
		Redactions: []secrets.Redaction{
			{Secret: "ghp_foo", RuleID: "github-pat"},
		},
	}}

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := applySecretDetection(e, nil, &out, &errOut, scanner, api.SecretDetectionActionCensor)
	if err != nil {
		t.Fatalf("applySecretDetection error: %v", err)
	}
	if string(e.ToolInput) != `{"content":"<SECRET:github-pat>"}` {
		t.Fatalf("tool input not redacted: %s", string(e.ToolInput))
	}
	if !e.SecretsDetected {
		t.Fatal("expected secrets_detected=true")
	}
	if e.Decision != hook.DecisionAllow {
		t.Fatalf("decision = %q, want allow", e.Decision)
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected deny output: %q", out.String())
	}
}

func TestApplySecretDetection_RedactsCommandFields(t *testing.T) {
	e := &hook.Event{
		ToolName:       "Bash",
		ToolInput:      json.RawMessage(`{"command":"echo \"sk_test_BQokikJOvBiI2HlWgH4olfQ2\""}`),
		CommandRaw:     `echo "sk_test_BQokikJOvBiI2HlWgH4olfQ2"`,
		CommandOpsJSON: `[{"type":"mutation","path":"x","raw":"echo sk_test_BQokikJOvBiI2HlWgH4olfQ2"}]`,
		Decision:       hook.DecisionAllow,
	}
	scanner := fakeSecretScanner{result: secrets.ScanResult{
		RedactedToolInput: json.RawMessage(`{"command":"echo \"<SECRET:stripe-access-token>\""}`),
		RuleIDs:           []string{"stripe-access-token"},
		Redactions: []secrets.Redaction{
			{Secret: "sk_test_BQokikJOvBiI2HlWgH4olfQ2", RuleID: "stripe-access-token"},
		},
	}}

	err := applySecretDetection(e, nil, &bytes.Buffer{}, &bytes.Buffer{}, scanner, api.SecretDetectionActionCensor)
	if err != nil {
		t.Fatalf("applySecretDetection error: %v", err)
	}
	if e.CommandRaw != `echo "<SECRET:stripe-access-token>"` {
		t.Fatalf("command_raw not redacted: %q", e.CommandRaw)
	}
	if e.CommandOpsJSON != `[{"type":"mutation","path":"x","raw":"echo <SECRET:stripe-access-token>"}]` {
		t.Fatalf("command_ops_json not redacted: %q", e.CommandOpsJSON)
	}
}

func TestApplySecretDetection_DenyBlocks(t *testing.T) {
	e := &hook.Event{ToolName: "Write", ToolInput: json.RawMessage(`{"content":"secret"}`), Decision: hook.DecisionAllow}
	scanner := fakeSecretScanner{result: secrets.ScanResult{RedactedToolInput: json.RawMessage(`{"content":"<SECRET:aws-access-key>"}`), RuleIDs: []string{"aws-access-key"}}}

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := applySecretDetection(e, nil, &out, &errOut, scanner, api.SecretDetectionActionDeny)
	if !errors.Is(err, hook.ErrDenied) {
		t.Fatalf("err = %v, want hook.ErrDenied", err)
	}
	if e.Decision != hook.DecisionDeny {
		t.Fatalf("decision = %q, want deny", e.Decision)
	}
	if e.DeniedOpReason != "secret detection policy violation" {
		t.Fatalf("DeniedOpReason = %q", e.DeniedOpReason)
	}
	if out.Len() == 0 {
		t.Fatal("expected deny response output")
	}
}

func TestApplySecretDetection_AllowDoesNotBlock(t *testing.T) {
	e := &hook.Event{ToolName: "Write", ToolInput: json.RawMessage(`{"content":"secret"}`), Decision: hook.DecisionAllow}
	scanner := fakeSecretScanner{result: secrets.ScanResult{RedactedToolInput: json.RawMessage(`{"content":"<SECRET:github-pat>"}`), RuleIDs: []string{"github-pat"}}}

	err := applySecretDetection(e, nil, &bytes.Buffer{}, &bytes.Buffer{}, scanner, api.SecretDetectionActionAllow)
	if err != nil {
		t.Fatalf("applySecretDetection error: %v", err)
	}
	if e.Decision != hook.DecisionAllow {
		t.Fatalf("decision = %q, want allow", e.Decision)
	}
	if string(e.ToolInput) != `{"content":"secret"}` {
		t.Fatalf("tool input should remain uncensored for allow, got: %s", string(e.ToolInput))
	}
	if e.SecretsDetected {
		t.Fatal("expected secrets_detected=false for allow mode")
	}
	if len(e.SecretRuleIDs) != 0 {
		t.Fatalf("expected no secret rule ids for allow mode, got: %v", e.SecretRuleIDs)
	}
}

func TestApplySecretDetection_PreservesExistingDeny(t *testing.T) {
	e := &hook.Event{ToolName: "Write", ToolInput: json.RawMessage(`{"content":"secret"}`), Decision: hook.DecisionDeny}
	scanner := fakeSecretScanner{result: secrets.ScanResult{RedactedToolInput: json.RawMessage(`{"content":"<SECRET:github-pat>"}`), RuleIDs: []string{"github-pat"}}}

	var out bytes.Buffer
	err := applySecretDetection(e, hook.ErrDenied, &out, &bytes.Buffer{}, scanner, api.SecretDetectionActionDeny)
	if !errors.Is(err, hook.ErrDenied) {
		t.Fatalf("err = %v, want hook.ErrDenied", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no extra deny output, got: %q", out.String())
	}
}
