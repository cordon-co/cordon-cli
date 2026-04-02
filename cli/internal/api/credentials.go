// Package api provides HTTP client and credential management for the cordon-web API.
package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// User represents an authenticated cordon user.
type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

// Credentials holds the stored authentication state.
type Credentials struct {
	AccessToken string    `json:"access_token"`
	ClientID    string    `json:"client_id,omitempty"`
	User        User      `json:"user"`
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// credentialsPath returns the path to ~/.cordon/credentials.json.
func credentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".cordon", "credentials.json"), nil
}

// LoadCredentials reads credentials from ~/.cordon/credentials.json.
// Returns nil (no error) if the file does not exist.
func LoadCredentials() (*Credentials, error) {
	p, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	return &c, nil
}

// SaveCredentials writes credentials to ~/.cordon/credentials.json with mode 0600.
func SaveCredentials(c *Credentials) error {
	p, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
}

// ClearCredentials deletes the credentials file.
func ClearCredentials() error {
	p, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove credentials: %w", err)
	}
	return nil
}

// IsLoggedIn returns true if credentials exist and haven't expired.
func IsLoggedIn() bool {
	c, err := LoadCredentials()
	if err != nil || c == nil {
		return false
	}
	return c.AccessToken != "" && time.Now().Before(c.ExpiresAt)
}

// EnsureClientID returns a stable client_id from credentials.json, generating
// and persisting one if missing.
func EnsureClientID() (string, error) {
	creds, err := LoadCredentials()
	if err != nil {
		return "", err
	}
	if creds == nil {
		return "", fmt.Errorf("no credentials found")
	}
	if creds.ClientID != "" {
		return creds.ClientID, nil
	}

	id, err := newClientID()
	if err != nil {
		return "", fmt.Errorf("generate client_id: %w", err)
	}
	creds.ClientID = id
	if err := SaveCredentials(creds); err != nil {
		return "", fmt.Errorf("persist client_id: %w", err)
	}
	return id, nil
}

func newClientID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// UUIDv4
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
