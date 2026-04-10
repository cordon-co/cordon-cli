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

func TestAnalyzeShellCommand_WrappedCommandResolvesInnerCd(t *testing.T) {
	a := analyzeShellCommand(`bash -lc 'cd scripts && cat README.md'`, "/repo")
	found := false
	for _, op := range a.Ops {
		if op.Type == shellOpRead && op.Path == "/repo/scripts/README.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected wrapped read op for /repo/scripts/README.md, ops=%+v", a.Ops)
	}
}

func TestExtractOpsFromArgv_SedInPlaceVariants(t *testing.T) {
	tests := [][]string{
		{"sed", "-i", "s/a/b/", "file.txt"},
		{"sed", "-i.bak", "s/a/b/", "file.txt"},
	}
	for _, argv := range tests {
		ops := extractOpsFromArgv(argv, "/repo")
		found := false
		for _, op := range ops {
			if op.Type == shellOpMutation && op.Path == "/repo/file.txt" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected sed in-place mutation for argv=%v, ops=%+v", argv, ops)
		}
	}
}

func TestExtractOpsFromArgv_CpWithFlagsAndDoubleDash(t *testing.T) {
	ops := extractOpsFromArgv([]string{"cp", "-r", "--", "src", "dst"}, "/repo")
	found := false
	for _, op := range ops {
		if op.Type == shellOpMutation && op.Path == "/repo/dst" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cp destination mutation /repo/dst, ops=%+v", ops)
	}
}

func TestExtractOpsFromArgv_TeeAppend(t *testing.T) {
	ops := extractOpsFromArgv([]string{"tee", "-a", "out.txt"}, "/repo")
	found := false
	for _, op := range ops {
		if op.Type == shellOpMutation && op.Path == "/repo/out.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected tee -a mutation /repo/out.txt, ops=%+v", ops)
	}
}
