package hook

import (
	"bytes"
	"strings"
	"testing"
)

func TestEvaluate_DirectFileDenyPopulatesReason(t *testing.T) {
	payload := `{
  "tool_name": "Write",
  "tool_input": {"file_path":"secret.txt","content":"x"},
  "cwd": "/repo"
}`

	checker := func(filePath, cwd string) (bool, string, bool) {
		return false, "", false
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	event, err := Evaluate(strings.NewReader(payload), &out, &errOut, checker, nil, nil)
	if err != ErrDenied {
		t.Fatalf("Evaluate error = %v, want ErrDenied", err)
	}
	if event == nil {
		t.Fatal("event = nil, want deny event")
	}
	if event.Decision != DecisionDeny {
		t.Fatalf("event.Decision = %q, want %q", event.Decision, DecisionDeny)
	}
	if event.DeniedOpIndex != -1 {
		t.Fatalf("event.DeniedOpIndex = %d, want -1", event.DeniedOpIndex)
	}
	if event.DeniedOpReason == "" {
		t.Fatal("event.DeniedOpReason is empty, want populated reason")
	}
	if event.DeniedOpReason != "prevent-write rule violation" {
		t.Fatalf("event.DeniedOpReason = %q, want prevent-write rule violation", event.DeniedOpReason)
	}
}

func TestEvaluate_ApplyPatchDenyPopulatesReason(t *testing.T) {
	payload := `{
  "tool_name": "apply_patch",
  "tool_input": {"input":"*** Begin Patch\n*** Update File: foo.txt\n+hi\n*** End Patch\n"},
  "cwd": "/repo"
}`

	checker := func(filePath, cwd string) (bool, string, bool) {
		if filePath == "foo.txt" {
			return false, "", false
		}
		return true, "", false
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	event, err := Evaluate(strings.NewReader(payload), &out, &errOut, checker, nil, nil)
	if err != ErrDenied {
		t.Fatalf("Evaluate error = %v, want ErrDenied", err)
	}
	if event == nil {
		t.Fatal("event = nil, want deny event")
	}
	if event.FilePath != "foo.txt" {
		t.Fatalf("event.FilePath = %q, want foo.txt", event.FilePath)
	}
	if event.DeniedOpIndex != -1 {
		t.Fatalf("event.DeniedOpIndex = %d, want -1", event.DeniedOpIndex)
	}
	if event.DeniedOpReason == "" {
		t.Fatal("event.DeniedOpReason is empty, want populated reason")
	}
	if event.DeniedOpReason != "prevent-write rule violation" {
		t.Fatalf("event.DeniedOpReason = %q, want prevent-write rule violation", event.DeniedOpReason)
	}
}
