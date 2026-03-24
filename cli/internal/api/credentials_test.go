package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Ensure directory is created by SaveCredentials.
	now := time.Now().UTC().Truncate(time.Second)
	creds := &Credentials{
		AccessToken: "test-token-123",
		User: User{
			ID:          "github|42",
			Username:    "testuser",
			DisplayName: "Test User",
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(30 * 24 * time.Hour),
	}

	if err := SaveCredentials(creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	// Verify file permissions.
	p := filepath.Join(tmp, ".cordon", "credentials.json")
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat credentials file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("credentials file permissions = %o, want 0600", perm)
	}

	// Load and verify.
	loaded, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadCredentials returned nil")
	}
	if loaded.AccessToken != creds.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, creds.AccessToken)
	}
	if loaded.User.Username != creds.User.Username {
		t.Errorf("Username = %q, want %q", loaded.User.Username, creds.User.Username)
	}
	if loaded.User.ID != creds.User.ID {
		t.Errorf("User.ID = %q, want %q", loaded.User.ID, creds.User.ID)
	}
	if !loaded.ExpiresAt.Equal(creds.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", loaded.ExpiresAt, creds.ExpiresAt)
	}
}

func TestLoadCredentials_NotExist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	creds, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds != nil {
		t.Errorf("expected nil credentials when file doesn't exist, got %+v", creds)
	}
}

func TestClearCredentials(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	creds := &Credentials{
		AccessToken: "token",
		User:        User{Username: "u"},
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := SaveCredentials(creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	if err := ClearCredentials(); err != nil {
		t.Fatalf("ClearCredentials: %v", err)
	}

	loaded, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials after clear: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil after ClearCredentials, got %+v", loaded)
	}
}

func TestClearCredentials_NotExist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Should not error when file doesn't exist.
	if err := ClearCredentials(); err != nil {
		t.Fatalf("ClearCredentials on missing file: %v", err)
	}
}

func TestIsLoggedIn(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// No credentials — not logged in.
	if IsLoggedIn() {
		t.Error("IsLoggedIn() = true with no credentials")
	}

	// Valid credentials.
	creds := &Credentials{
		AccessToken: "token",
		User:        User{Username: "u"},
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := SaveCredentials(creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	if !IsLoggedIn() {
		t.Error("IsLoggedIn() = false with valid credentials")
	}

	// Expired credentials.
	creds.ExpiresAt = time.Now().Add(-time.Hour)
	if err := SaveCredentials(creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	if IsLoggedIn() {
		t.Error("IsLoggedIn() = true with expired credentials")
	}
}
