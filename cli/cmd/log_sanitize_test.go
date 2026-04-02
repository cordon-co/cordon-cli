package cmd

import "testing"

func TestSanitizeRepoPathInJSONStrings(t *testing.T) {
	absRoot := "/Users/tom/Projects/cordon"
	in := `{"command":"cd /Users/tom/Projects/cordon && go test ./...","paths":["/Users/tom/Projects/cordon/cli/cmd/hook.go",42]}`
	got := sanitizeRepoPathInJSONStrings(in, absRoot)
	want := `{"command":"cd /<REPO_PATH> && go test ./...","paths":["/<REPO_PATH>/cli/cmd/hook.go",42]}`
	if got != want {
		t.Fatalf("sanitizeRepoPathInJSONStrings() = %s\nwant %s", got, want)
	}
}

func TestSanitizeRepoPathInString(t *testing.T) {
	absRoot := "/Users/tom/Projects/cordon"
	in := `[{"raw":"cd /Users/tom/Projects/cordon && cat /Users/tom/Projects/cordon/README.md"}]`
	got := sanitizeRepoPathInString(in, absRoot)
	want := `[{"raw":"cd /<REPO_PATH> && cat /<REPO_PATH>/README.md"}]`
	if got != want {
		t.Fatalf("sanitizeRepoPathInString() = %s\nwant %s", got, want)
	}
}

func TestSanitizeRepoPathInJSONStrings_InvalidJSONFallback(t *testing.T) {
	absRoot := "/Users/tom/Projects/cordon"
	in := `cd /Users/tom/Projects/cordon && gofmt -w`
	got := sanitizeRepoPathInJSONStrings(in, absRoot)
	want := `cd /<REPO_PATH> && gofmt -w`
	if got != want {
		t.Fatalf("fallback sanitize = %s, want %s", got, want)
	}
}
