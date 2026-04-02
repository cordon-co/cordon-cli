package auth

import (
	"encoding/json"
	"fmt"

	"github.com/cordon-co/cordon-cli/cli/internal/api"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored credentials",
	Long:  "Revokes the current token server-side and removes local credentials.",
	Args:  cobra.NoArgs,
	RunE:  RunLogout,
}

type logoutResult struct {
	LoggedOut bool `json:"logged_out"`
}

// RunLogout implements the logout flow. Exported for use as a top-level alias.
func RunLogout(cmd *cobra.Command, args []string) error {
	creds, err := api.LoadCredentials()
	if err != nil {
		return fmt.Errorf("auth logout: %w", err)
	}
	if creds == nil {
		if flags.JSON {
			out, _ := json.MarshalIndent(logoutResult{LoggedOut: false}, "", "  ")
			fmt.Println(string(out))
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Not logged in.")
		return nil
	}

	// Best-effort server-side revocation.
	client := api.NewClientWithToken(creds.AccessToken)
	_, _ = client.PostJSON("/api/v1/auth/revoke", nil, nil)

	if err := api.ClearCredentials(); err != nil {
		return fmt.Errorf("auth logout: %w", err)
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(logoutResult{LoggedOut: true}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Logged out.")
	return nil
}
