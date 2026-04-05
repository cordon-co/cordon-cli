package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Standard API errors returned by the client.
var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
)

// APIError represents a structured error response from the server.
type APIError struct {
	StatusCode int
	Code       string `json:"error"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("API error %d", e.StatusCode)
}

// Is allows errors.Is to match APIError against sentinel errors.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return e.StatusCode == 401
	case ErrForbidden:
		return e.StatusCode == 403
	case ErrNotFound:
		return e.StatusCode == 404
	}
	return false
}

// Client is an authenticated HTTP client for the cordon-web API.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// configFile represents ~/.cordon/config.json.
type configFile struct {
	APIURL                string `json:"api_url"`
	SecretDetectionAction string `json:"secret_detection_action"`
}

const (
	SecretDetectionActionCensor = "censor"
	SecretDetectionActionDeny   = "deny"
	SecretDetectionActionAllow  = "allow"
)

// resolveBaseURL returns the API base URL from env, config file, or default.
func resolveBaseURL() string {
	if v := os.Getenv("CORDON_API_URL"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err == nil {
		data, err := os.ReadFile(filepath.Join(home, ".cordon", "config.json"))
		if err == nil {
			var cfg configFile
			if json.Unmarshal(data, &cfg) == nil && cfg.APIURL != "" {
				return cfg.APIURL
			}
		}
	}
	return "https://api.cordon.sh"
}

// NewClient creates an API client using the token resolution chain:
// CORDON_TOKEN env var > stored credentials.
func NewClient() (*Client, error) {
	c := &Client{
		BaseURL:    resolveBaseURL(),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
	token, _, err := ResolveToken()
	if err != nil {
		return nil, fmt.Errorf("resolve token: %w", err)
	}
	c.Token = token
	return c, nil
}

// NewClientWithToken creates an API client with the given token (no credential file read).
func NewClientWithToken(token string) *Client {
	return &Client{
		BaseURL:    resolveBaseURL(),
		Token:      token,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewUnauthenticatedClient creates an API client with no token.
func NewUnauthenticatedClient() *Client {
	return &Client{
		BaseURL:    resolveBaseURL(),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Do sends an HTTP request and checks for token refresh headers.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("User-Agent", "cordon-cli")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Check for token refresh via response header (only for OAuth tokens).
	if newToken := resp.Header.Get("X-Cordon-Token"); newToken != "" && !isMachineToken(c.Token) {
		c.Token = newToken
		// Best-effort update of stored credentials.
		if creds, err := LoadCredentials(); err == nil && creds != nil {
			creds.AccessToken = newToken
			_ = SaveCredentials(creds)
		}
	}

	return resp, nil
}

// PostJSON sends a POST request with a JSON body and decodes the response.
func (c *Client) PostJSON(path string, reqBody, respBody any) (*http.Response, error) {
	var body io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest("POST", c.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return resp, parseErrorResponse(resp)
	}

	if respBody != nil && resp.StatusCode != http.StatusNoContent {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return resp, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp, nil
}

// GetJSON sends a GET request and decodes the JSON response.
func (c *Client) GetJSON(path string, respBody any) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return resp, parseErrorResponse(resp)
	}

	if respBody != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return resp, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp, nil
}

func parseErrorResponse(resp *http.Response) error {
	defer resp.Body.Close()
	var apiErr APIError
	apiErr.StatusCode = resp.StatusCode
	data, err := io.ReadAll(resp.Body)
	if err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &apiErr)
	}
	return &apiErr
}

// ReadConfigURL is exported for testing — returns the base URL from config file only.
func ReadConfigURL() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".cordon", "config.json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ""
		}
		return ""
	}
	var cfg configFile
	if json.Unmarshal(data, &cfg) == nil {
		return cfg.APIURL
	}
	return ""
}

// isMachineToken returns true if the token has the machine token prefix.
func isMachineToken(token string) bool {
	return len(token) > 4 && token[:4] == "cmt_"
}

// ReadSecretDetectionAction returns the secret detection action from
// ~/.cordon/config.json. Missing, unreadable, malformed, or invalid values
// default to "censor".
func ReadSecretDetectionAction() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return SecretDetectionActionCensor
	}

	data, err := os.ReadFile(filepath.Join(home, ".cordon", "config.json"))
	if err != nil {
		return SecretDetectionActionCensor
	}

	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return SecretDetectionActionCensor
	}

	switch cfg.SecretDetectionAction {
	case SecretDetectionActionCensor, SecretDetectionActionDeny, SecretDetectionActionAllow:
		return cfg.SecretDetectionAction
	default:
		return SecretDetectionActionCensor
	}
}
