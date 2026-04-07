package tui

import (
	"testing"

	"github.com/cordon-co/cordon-cli/cli/internal/store"
)

func TestLiveActionSummaryVerbByDecision(t *testing.T) {
	tests := []struct {
		name  string
		entry store.UnifiedEntry
		want  string
	}{
		{
			name: "allow uses used verb",
			entry: store.UnifiedEntry{
				EventType: "hook_allow",
				ToolName:  "run_in_terminal",
				FilePath:  "echo hello",
				Agent:     "claude-code",
			},
			want: "claude-code used run_in_terminal on file echo hello",
		},
		{
			name: "deny uses attempted verb",
			entry: store.UnifiedEntry{
				EventType: "hook_deny",
				ToolName:  "run_in_terminal",
				FilePath:  "echo <SECRET:stripe-access-token>",
				Agent:     "claude-code",
			},
			want: "claude-code attempted run_in_terminal on file echo <SECRET:stripe-access-token>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := liveActionSummary(tt.entry)
			if got != tt.want {
				t.Fatalf("liveActionSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}
