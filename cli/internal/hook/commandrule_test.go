package hook

import (
	"reflect"
	"testing"
)

func TestSplitCompoundCommand_QuotedDelimiters(t *testing.T) {
	got := splitCompoundCommand(`echo "a && b ; c | d" && git status`)
	want := []string{`echo "a && b ; c | d"`, "git status"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
}

func TestSplitCompoundCommand_NestedAndPipeline(t *testing.T) {
	got := splitCompoundCommand(`cd /tmp && (git status; git add a.txt) | cat && echo done`)
	want := []string{"cd /tmp", "git status", "git add a.txt", "cat", "echo done"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
}

func TestSplitCompoundCommand_ParseFailureFallsBackToRaw(t *testing.T) {
	got := splitCompoundCommand(`echo "unterminated`)
	want := []string{`echo "unterminated`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
}

func TestSplitCompoundCommand_NoCallExpressionFallsBackToRaw(t *testing.T) {
	got := splitCompoundCommand(`FOO=bar`)
	want := []string{"FOO=bar"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
}
