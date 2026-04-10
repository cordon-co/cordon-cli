package tests

import (
	"bytes"
	"os/exec"
	"testing"
)

func runHookWithPayload(t *testing.T, repo testRepo, payload string) runResult {
	t.Helper()
	cmd := exec.Command(binaryPath, "hook")
	cmd.Dir = repo.Dir
	cmd.Env = repo.env()
	cmd.Stdin = bytes.NewBufferString(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	r := runResult{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			r.ExitCode = ee.ExitCode()
		} else {
			t.Fatalf("exec cordon hook: %v", err)
		}
	}
	return r
}

func TestHook_CommandAllowOverridesDenyForWrappedCommand(t *testing.T) {
	repo := initRepo(t)

	runCordon(t, repo, "command", "add", "git commit")
	runCordon(t, repo, "command", "add", "git commit *", "--allow")

	payload := `{"tool_name":"bash","tool_input":{"command":"sh -c 'git commit -m \"test\"'"},"cwd":"` + repo.Dir + `"}`
	r := runHookWithPayload(t, repo, payload)
	if r.ExitCode != 0 {
		t.Fatalf("hook exit = %d, want 0\nstdout:%s\nstderr:%s", r.ExitCode, r.Stdout, r.Stderr)
	}
}

func TestHook_CommandDenyForWrappedCommandWithoutAllow(t *testing.T) {
	repo := initRepo(t)

	runCordon(t, repo, "command", "add", "git commit")

	payload := `{"tool_name":"bash","tool_input":{"command":"sh -c 'git commit -m \"test\"'"},"cwd":"` + repo.Dir + `"}`
	r := runHookWithPayload(t, repo, payload)
	if r.ExitCode != 2 {
		t.Fatalf("hook exit = %d, want 2\nstdout:%s\nstderr:%s", r.ExitCode, r.Stdout, r.Stderr)
	}
}
