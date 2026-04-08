package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/store"
)

func TestResolveExportPath(t *testing.T) {
	t.Setenv("HOME", "/tmp/cordon-home")

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{
			name: "absolute path is unchanged",
			in:   "/tmp/out.csv",
			want: "/tmp/out.csv",
		},
		{
			name: "expands tilde path",
			in:   "~/out.csv",
			want: "/tmp/cordon-home/out.csv",
		},
		{
			name:    "rejects empty path",
			in:      " ",
			wantErr: "file path cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveExportPath(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("resolveExportPath(%q) expected error %q, got nil", tt.in, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveExportPath(%q) error = %q, want contains %q", tt.in, err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveExportPath(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("resolveExportPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestWriteLogCSVFile_WritesCSV(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "audit.csv")
	entries := []store.UnifiedEntry{
		{
			Time:      time.Date(2026, 3, 22, 10, 11, 12, 0, time.UTC),
			EventType: "hook_allow",
			ToolName:  "Write",
			FilePath:  "cli/cmd/log.go",
			Agent:     "claude-code",
		},
	}

	if err := writeLogCSVFile(outPath, entries); err != nil {
		t.Fatalf("writeLogCSVFile() error = %v", err)
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", outPath, err)
	}
	content := string(b)
	if !strings.Contains(content, "timestamp,event_type,tool_name,file_path,file_rule_id,pass_id,user,agent,session_id,detail") {
		t.Fatalf("CSV content missing header: %q", content)
	}
	if !strings.Contains(content, "2026-03-22T10:11:12Z,hook_allow,Write,cli/cmd/log.go,,,,claude-code,,") {
		t.Fatalf("CSV content missing row: %q", content)
	}
}

func TestWriteLogCSVFile_ExpandsHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	outPath := "~/log-export.csv"

	if err := writeLogCSVFile(outPath, nil); err != nil {
		t.Fatalf("writeLogCSVFile(%q) error = %v", outPath, err)
	}

	expanded := filepath.Join(home, "log-export.csv")
	if _, err := os.Stat(expanded); err != nil {
		t.Fatalf("expected file at %q: %v", expanded, err)
	}
}
