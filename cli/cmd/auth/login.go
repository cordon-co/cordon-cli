package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/api"
	"github.com/cordon-co/cordon-cli/cli/internal/apicontract"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate via GitHub OAuth",
	Long:  "Starts a device OAuth flow — opens a browser to complete GitHub authorization and stores credentials in ~/.cordon/credentials.json.",
	Args:  cobra.NoArgs,
	RunE:  RunLogin,
}

type deviceResponse = apicontract.DeviceCodeResponse
type tokenResponse = apicontract.TokenResponse

type loginResult struct {
	User      api.User  `json:"user"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RunLogin implements the login flow. Exported for use as a top-level alias.
func RunLogin(cmd *cobra.Command, args []string) error {
	// Check if already logged in.
	if api.IsLoggedIn() {
		creds, _ := api.LoadCredentials()
		if creds != nil {
			if flags.JSON {
				out, _ := json.MarshalIndent(loginResult{
					User:      creds.User,
					ExpiresAt: creds.ExpiresAt,
				}, "", "  ")
				fmt.Println(string(out))
				return nil
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Already logged in as %s. Run \"cordon auth logout\" first to switch accounts.\n", creds.User.Username)
			return nil
		}
	}

	client := api.NewUnauthenticatedClient()

	// Step 1: Start device flow.
	var device deviceResponse
	_, err := client.PostJSON("/api/v1/auth/device", map[string]string{"client_id": "cordon-cli"}, &device)
	if err != nil {
		return fmt.Errorf("auth login: start device flow: %w", err)
	}

	// Step 2: Display code and open browser.
	if !flags.JSON {
		fmt.Fprintf(cmd.OutOrStdout(), "\nOpen this URL in your browser: %s\n", device.VerificationUri)
		fmt.Fprintf(cmd.OutOrStdout(), "Enter code: %s\n\n", device.UserCode)
		fmt.Fprintln(cmd.OutOrStdout(), "Waiting for authorization...")
	}
	openBrowser(device.VerificationUri)

	// Step 3: Poll for token.
	interval := time.Duration(device.Interval) * time.Second
	if interval < 1*time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(device.ExpiresIn) * time.Second)

	tokenReq := map[string]string{
		"device_code": device.DeviceCode,
		"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
	}

	for time.Now().Before(deadline) {
		time.Sleep(interval)

		var token tokenResponse
		_, err := client.PostJSON("/api/v1/auth/token", tokenReq, &token)
		if err != nil {
			var apiErr *api.APIError
			if errors.As(err, &apiErr) {
				switch apiErr.Code {
				case "authorization_pending":
					continue
				case "access_denied":
					return fmt.Errorf("auth login: authorization denied by user")
				case "expired_token":
					return fmt.Errorf("auth login: device code expired, please try again")
				}
			}
			// For non-API errors (network issues), keep polling.
			continue
		}

		// Success — save credentials.
		now := time.Now().UTC()
		user := api.User{
			ID:          token.User.Id,
			Username:    token.User.Username,
			DisplayName: token.User.DisplayName,
		}
		creds := &api.Credentials{
			AccessToken: token.AccessToken,
			User:        user,
			IssuedAt:    now,
			ExpiresAt:   now.Add(time.Duration(token.ExpiresIn) * time.Second),
		}
		if err := api.SaveCredentials(creds); err != nil {
			return fmt.Errorf("auth login: save credentials: %w", err)
		}

		if flags.JSON {
			out, _ := json.MarshalIndent(loginResult{
				User:      creds.User,
				ExpiresAt: creds.ExpiresAt,
			}, "", "  ")
			fmt.Println(string(out))
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s\n", creds.User.Username)
		return nil
	}

	return fmt.Errorf("auth login: device code expired, please try again")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}
