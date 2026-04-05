package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/api"
	"github.com/cordon-co/cordon-cli/cli/internal/apicontract"
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

type perimeter struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type statusResult struct {
	Authenticated bool        `json:"authenticated"`
	AuthType      string      `json:"auth_type,omitempty"`
	TokenName     string      `json:"token_name,omitempty"`
	User          *api.User   `json:"user,omitempty"`
	Perimeters    []perimeter `json:"perimeters,omitempty"`
	ExpiresAt     *time.Time  `json:"expires_at,omitempty"`
}

// meWithTokenName extends the standard MeResponse with an optional token_name field.
type meWithTokenName struct {
	apicontract.MeResponse
	TokenName string `json:"token_name"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	token, tokenType, err := api.ResolveToken()
	if err != nil {
		return fmt.Errorf("auth status: %w", err)
	}
	if token == "" {
		if flags.JSON {
			out, _ := json.MarshalIndent(statusResult{Authenticated: false}, "", "  ")
			fmt.Println(string(out))
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Not authenticated. Run \"cordon auth login\" to log in.")
		return nil
	}

	// Load stored credentials for metadata (token name, expiry).
	creds, _ := api.LoadCredentials()

	// Verify token with server.
	client := api.NewClientWithToken(token)
	var me meWithTokenName
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

	user := api.User{
		ID:          me.User.Id,
		Username:    me.User.Username,
		DisplayName: me.User.DisplayName,
	}
	perimeters := make([]perimeter, 0, len(me.Perimeters))
	for _, p := range me.Perimeters {
		perimeters = append(perimeters, perimeter{
			ID:   p.Id,
			Name: p.Name,
			Role: string(p.Role),
		})
	}

	// Determine token name: prefer server response, fall back to stored creds.
	tokenName := me.TokenName
	if tokenName == "" && creds != nil {
		tokenName = creds.TokenName
	}

	if flags.JSON {
		result := statusResult{
			Authenticated: true,
			AuthType:      tokenType,
			User:          &user,
			Perimeters:    perimeters,
		}
		if tokenType == api.CredentialTypeMachine {
			result.TokenName = tokenName
		} else if creds != nil {
			result.ExpiresAt = &creds.ExpiresAt
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s", user.Username)
	if user.DisplayName != "" && user.DisplayName != user.Username {
		fmt.Fprintf(cmd.OutOrStdout(), " (%s)", user.DisplayName)
	}
	fmt.Fprintln(cmd.OutOrStdout())

	if tokenType == api.CredentialTypeMachine {
		fmt.Fprintf(cmd.OutOrStdout(), "Auth type: machine token")
		if tokenName != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " (%s)", tokenName)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Auth type: OAuth")
	}

	if len(perimeters) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nPerimeters:")
		for _, p := range perimeters {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s (%s)\n", p.Name, p.Role)
		}
	}

	if tokenType != api.CredentialTypeMachine && creds != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "\nToken expires: %s\n", creds.ExpiresAt.Format(time.RFC3339))
	}
	return nil
}
