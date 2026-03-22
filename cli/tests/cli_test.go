// Package tests contains end-to-end integration tests that build the cordon
// binary and exercise it via subprocesses. Each test gets its own isolated
// temporary directory so tests do not interfere with each other or the host.
package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// binaryPath is set by TestMain to the path of the compiled cordon binary.
var binaryPath string

// TestMain builds the cordon binary once before all tests run.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "cordon-bin-*")
	if err != nil {
		panic("create temp dir for binary: " + err.Error())
	}
	defer os.RemoveAll(tmp)

	binaryPath = filepath.Join(tmp, "cordon")

	// Build from the repo root (two levels up from tests/).
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		panic("resolve repo root: " + err.Error())
	}
	out, err := exec.Command("go", "build", "-o", binaryPath, filepath.Join(repoRoot, "cmd", "cordon")).CombinedOutput()
	if err != nil {
		panic("build cordon binary: " + err.Error() + "\n" + string(out))
	}

	os.Exit(m.Run())
}

// testRepo holds the isolated directories for a single test run.
type testRepo struct {
	Dir  string // repo directory (where .cordon/ lives)
	Home string // home directory (where ~/.cordon/ lives)
}

// env returns an os.Environ slice with HOME replaced by the test home directory.
func (r testRepo) env() []string {
	var env []string
	for _, e := range os.Environ() {
		if len(e) >= 5 && e[:5] == "HOME=" {
			continue
		}
		env = append(env, e)
	}
	return append(env, "HOME="+r.Home)
}

// initRepo creates isolated repo and home directories, runs `cordon init --json`,
// and returns a testRepo. All subsequent commands should use the same testRepo.
func initRepo(t *testing.T) testRepo {
	t.Helper()
	repo := testRepo{
		Dir:  t.TempDir(),
		Home: t.TempDir(),
	}
	runCordonIn(t, repo, "init", "--json")
	return repo
}

// runResult holds the captured output of a cordon invocation.
type runResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// runCordon executes the cordon binary in the repo and fatally fails the test
// if the exit code is non-zero.
func runCordon(t *testing.T, repo testRepo, args ...string) runResult {
	t.Helper()
	r := runCordonIn(t, repo, args...)
	if r.ExitCode != 0 {
		t.Fatalf("cordon %v failed (exit %d):\nstdout: %s\nstderr: %s",
			args, r.ExitCode, r.Stdout, r.Stderr)
	}
	return r
}

// runCordonRaw executes the cordon binary and returns the result without
// failing the test. Use when a non-zero exit code is expected.
func runCordonRaw(t *testing.T, repo testRepo, args ...string) runResult {
	return runCordonIn(t, repo, args...)
}

func runCordonIn(t *testing.T, repo testRepo, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = repo.Dir
	cmd.Env = repo.env()
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
			t.Fatalf("exec cordon %v: %v", args, err)
		}
	}
	return r
}

// mustParseJSON parses JSON from data into v, failing the test on error.
func mustParseJSON(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("parse JSON: %v\ndata: %s", err, data)
	}
}
