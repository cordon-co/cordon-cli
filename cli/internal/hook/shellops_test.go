package hook

import (
	"strings"
	"testing"
)

func TestAnalyzeShellCommand_CwdAwareReadPath(t *testing.T) {
	a := analyzeShellCommand("cd scripts && cat README.md", "/repo")
	if len(a.Ops) == 0 {
		t.Fatal("expected ops, got none")
	}

	found := false
	for _, op := range a.Ops {
		if op.Type == shellOpRead && op.Path == "/repo/scripts/README.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected read op for /repo/scripts/README.md, ops=%+v", a.Ops)
	}
}

func TestAnalyzeShellCommand_GitMutations(t *testing.T) {
	a := analyzeShellCommand("git add a.txt b.txt && git commit -a", "/repo")
	var paths []string
	for _, op := range a.Ops {
		if op.Type == shellOpMutation {
			paths = append(paths, op.Path)
		}
	}

	want := []string{"/repo/a.txt", "/repo/b.txt", "/repo"}
	for _, w := range want {
		ok := false
		for _, p := range paths {
			if p == w {
				ok = true
				break
			}
		}
		if !ok {
			t.Fatalf("missing mutation path %q in %+v", w, paths)
		}
	}
}

func TestAnalyzeShellCommand_AmbiguityOnExpansion(t *testing.T) {
	a := analyzeShellCommand("cat \"$FILE\"", "/repo")
	if !strings.Contains(a.ambiguityText(), "argv_unresolved") {
		t.Fatalf("ambiguity = %q, want argv_unresolved", a.ambiguityText())
	}
}
