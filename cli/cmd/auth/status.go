package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/api"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	Long:  "Verifies the stored token against the server and displays current user info.",
	Args:  cobra.NoArgs,
	RunE:  runStatus,
}

// meResponse is the response from GET /api/v1/auth/me.
type meResponse struct {
	User       api.User    `json:"user"`
	Perimeters []perimeter `json:"perimeters"`
}

type perimeter struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type statusResult struct {
	Authenticated bool        `json:"authenticated"`
	User          *api.User   `json:"user,omitempty"`
	Perimeters    []perimeter `json:"perimeters,omitempty"`
	ExpiresAt     *time.Time  `json:"expires_at,omitempty"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	creds, err := api.LoadCredentials()
	if err != nil {
		return fmt.Errorf("auth status: %w", err)
	}
	if creds == nil || creds.AccessToken == "" {
		if flags.JSON {
			out, _ := json.MarshalIndent(statusResult{Authenticated: false}, "", "  ")
			fmt.Println(string(out))
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Not authenticated. Run \"cordon auth login\" to log in.")
		return nil
	}

	// Verify token with server.
	client := api.NewClientWithToken(creds.AccessToken)
	var me meResponse
	_, err = client.GetJSON("/api/v1/auth/me", &me)
	if err != nil {
		if errors.Is(err, api.ErrUnauthorized) {
			// Token expired or revoked — clear stale credentials.
			_ = api.ClearCredentials()
			if flags.JSON {
				out, _ := json.MarshalIndent(statusResult{Authenticated: false}, "", "  ")
				fmt.Println(string(out))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Session expired. Run \"cordon auth login\" to re-authenticate.")
			return nil
		}
		return fmt.Errorf("auth status: verify token: %w", err)
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(statusResult{
			Authenticated: true,
			User:          &me.User,
			Perimeters:    me.Perimeters,
			ExpiresAt:     &creds.ExpiresAt,
		}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s", me.User.Username)
	if me.User.DisplayName != "" && me.User.DisplayName != me.User.Username {
		fmt.Fprintf(cmd.OutOrStdout(), " (%s)", me.User.DisplayName)
	}
	fmt.Fprintln(cmd.OutOrStdout())

	if len(me.Perimeters) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nPerimeters:")
		for _, p := range me.Perimeters {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s (%s)\n", p.Name, p.Role)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nToken expires: %s\n", creds.ExpiresAt.Format(time.RFC3339))
	return nil
}
