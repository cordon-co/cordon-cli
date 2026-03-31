package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveBaseURL_EnvVar(t *testing.T) {
	t.Setenv("CORDON_API_URL", "http://localhost:9999")
	got := resolveBaseURL()
	if got != "http://localhost:9999" {
		t.Errorf("resolveBaseURL() = %q, want http://localhost:9999", got)
	}
}

func TestResolveBaseURL_Default(t *testing.T) {
	t.Setenv("CORDON_API_URL", "")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	got := resolveBaseURL()
	if got != "https://api.cordon.sh" {
		t.Errorf("resolveBaseURL() = %q, want https://api.cordon.sh", got)
	}
}

func TestClient_GetJSON(t *testing.T) {
	type payload struct {
		Message string `json:"message"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/test" {
			t.Errorf("path = %s, want /api/v1/test", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload{Message: "hello"})
	}))
	defer srv.Close()

	client := &Client{
		BaseURL:    srv.URL,
		Token:      "test-token",
		HTTPClient: srv.Client(),
	}

	var resp payload
	_, err := client.GetJSON("/api/v1/test", &resp)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if resp.Message != "hello" {
		t.Errorf("Message = %q, want hello", resp.Message)
	}
}

func TestClient_PostJSON(t *testing.T) {
	type reqBody struct {
		Name string `json:"name"`
	}
	type respBody struct {
		ID int `json:"id"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var req reqBody
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name != "test" {
			t.Errorf("req.Name = %q, want test", req.Name)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody{ID: 42})
	}))
	defer srv.Close()

	client := &Client{
		BaseURL:    srv.URL,
		Token:      "tok",
		HTTPClient: srv.Client(),
	}

	var resp respBody
	_, err := client.PostJSON("/api/v1/create", reqBody{Name: "test"}, &resp)
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if resp.ID != 42 {
		t.Errorf("ID = %d, want 42", resp.ID)
	}
}

func TestClient_ErrorResponses(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		body         string
		wantSentinel error
		wantCode     string
	}{
		{
			name:         "401 unauthorized",
			status:       401,
			body:         `{"error":"token_expired","message":"JWT has expired"}`,
			wantSentinel: ErrUnauthorized,
			wantCode:     "token_expired",
		},
		{
			name:         "403 forbidden",
			status:       403,
			body:         `{"error":"access_denied"}`,
			wantSentinel: ErrForbidden,
			wantCode:     "access_denied",
		},
		{
			name:         "404 not found",
			status:       404,
			body:         `{"error":"perimeter_not_found","message":"No perimeter registered"}`,
			wantSentinel: ErrNotFound,
			wantCode:     "perimeter_not_found",
		},
		{
			name:     "428 pending",
			status:   428,
			body:     `{"error":"authorization_pending"}`,
			wantCode: "authorization_pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			client := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
			_, err := client.GetJSON("/test", nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *APIError, got %T: %v", err, err)
			}
			if apiErr.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", apiErr.Code, tt.wantCode)
			}
			if apiErr.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.status)
			}
			if tt.wantSentinel != nil && !errors.Is(err, tt.wantSentinel) {
				t.Errorf("errors.Is(err, %v) = false", tt.wantSentinel)
			}
		})
	}
}

func TestClient_TokenRefresh(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Save initial credentials so the refresh can update them.
	initial := &Credentials{
		AccessToken: "old-token",
		User:        User{Username: "u"},
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := SaveCredentials(initial); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Cordon-Token", "refreshed-token")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := &Client{
		BaseURL:    srv.URL,
		Token:      "old-token",
		HTTPClient: srv.Client(),
	}

	var resp map[string]bool
	_, err := client.GetJSON("/test", &resp)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}

	// Client token should be updated.
	if client.Token != "refreshed-token" {
		t.Errorf("client.Token = %q, want refreshed-token", client.Token)
	}

	// Stored credentials should be updated.
	creds, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds.AccessToken != "refreshed-token" {
		t.Errorf("stored token = %q, want refreshed-token", creds.AccessToken)
	}
}

func TestAPIError_Is(t *testing.T) {
	err := &APIError{StatusCode: 401, Code: "token_expired"}
	if !errors.Is(err, ErrUnauthorized) {
		t.Error("401 should match ErrUnauthorized")
	}
	if errors.Is(err, ErrForbidden) {
		t.Error("401 should not match ErrForbidden")
	}

	err403 := &APIError{StatusCode: 403, Code: "access_denied"}
	if !errors.Is(err403, ErrForbidden) {
		t.Error("403 should match ErrForbidden")
	}
}

func TestReadSecretDetectionAction_DefaultsToCensor(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if got := ReadSecretDetectionAction(); got != SecretDetectionActionCensor {
		t.Fatalf("ReadSecretDetectionAction() = %q, want %q", got, SecretDetectionActionCensor)
	}
}

func TestReadSecretDetectionAction_ParsesKnownValues(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, ".cordon"), 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		value string
		want  string
	}{
		{value: SecretDetectionActionCensor, want: SecretDetectionActionCensor},
		{value: SecretDetectionActionDeny, want: SecretDetectionActionDeny},
		{value: SecretDetectionActionAllow, want: SecretDetectionActionAllow},
	}
	for _, tt := range tests {
		data := []byte(`{"secret_detection_action":"` + tt.value + `"}`)
		if err := os.WriteFile(filepath.Join(tmp, ".cordon", "config.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := ReadSecretDetectionAction(); got != tt.want {
			t.Fatalf("value=%q got %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestReadSecretDetectionAction_InvalidDefaultsToCensor(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, ".cordon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".cordon", "config.json"), []byte(`{"secret_detection_action":"bad"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ReadSecretDetectionAction(); got != SecretDetectionActionCensor {
		t.Fatalf("ReadSecretDetectionAction() = %q, want %q", got, SecretDetectionActionCensor)
	}
}
