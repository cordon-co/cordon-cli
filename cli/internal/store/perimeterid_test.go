package store

import (
	"testing"
)

func TestNormalizeRemoteURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "SSH URL",
			input: "git@github.com:org/repo.git",
			want:  "github.com/org/repo",
		},
		{
			name:  "HTTPS URL with .git",
			input: "https://github.com/org/repo.git",
			want:  "github.com/org/repo",
		},
		{
			name:  "HTTPS URL without .git",
			input: "https://github.com/org/repo",
			want:  "github.com/org/repo",
		},
		{
			name:  "SSH URL without .git",
			input: "git@github.com:Org/Repo",
			want:  "github.com/org/repo",
		},
		{
			name:  "mixed case",
			input: "https://GitHub.COM/ORG/REPO.git",
			want:  "github.com/org/repo",
		},
		{
			name:  "trailing slash",
			input: "https://github.com/org/repo/",
			want:  "github.com/org/repo",
		},
		{
			name:  "git protocol",
			input: "git://github.com/org/repo.git",
			want:  "github.com/org/repo",
		},
		{
			name:  "HTTP URL",
			input: "http://gitlab.example.com/team/project.git",
			want:  "gitlab.example.com/team/project",
		},
		{
			name:  "SSH with nested path",
			input: "git@gitlab.com:group/subgroup/repo.git",
			want:  "gitlab.com/group/subgroup/repo",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:  "whitespace trimmed",
			input: "  https://github.com/org/repo.git  ",
			want:  "github.com/org/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeRemoteURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("NormalizeRemoteURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDeriveRemotePerimeterID(t *testing.T) {
	// Same input should always produce the same output.
	id1, err := DeriveRemotePerimeterID("git@github.com:org/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := DeriveRemotePerimeterID("https://github.com/org/repo.git")
	if err != nil {
		t.Fatal(err)
	}

	if id1 != id2 {
		t.Errorf("same repo via SSH and HTTPS produced different IDs: %s vs %s", id1, id2)
	}

	// Length should be 32 hex chars.
	if len(id1) != 32 {
		t.Errorf("expected 32-char hex ID, got %d chars: %s", len(id1), id1)
	}

	// Different repos should produce different IDs.
	id3, err := DeriveRemotePerimeterID("git@github.com:other/project.git")
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id3 {
		t.Error("different repos produced the same ID")
	}

	// Case insensitive.
	id4, err := DeriveRemotePerimeterID("git@GitHub.COM:ORG/REPO.git")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id4 {
		t.Errorf("case-different URLs produced different IDs: %s vs %s", id1, id4)
	}
}
