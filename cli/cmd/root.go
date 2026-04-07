// Package cmd implements the cordon CLI command tree.
package cmd

import (
	"context"
	"os"

	"github.com/cordon-co/cordon-cli/cli/cmd/auth"
	"github.com/cordon-co/cordon-cli/cli/cmd/command"
	"github.com/cordon-co/cordon-cli/cli/cmd/file"
	"github.com/cordon-co/cordon-cli/cli/cmd/pass"
	"github.com/cordon-co/cordon-cli/cli/internal/buildinfo"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/cordon-co/cordon-cli/cli/internal/mcpserver"
	"github.com/cordon-co/cordon-cli/cli/internal/updatecheck"
	"github.com/spf13/cobra"
)

var mcpMode bool

var rootCmd = &cobra.Command{
	Use:   "cordon",
	Short: "Team-wide access policies for AI coding agents",
	Long: `Cordon enforces policy restrictions on AI agents. File and command rules define protected targets; passes grant
temporary access; the audit log captures every enforcement decision.`,
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	// Root RunE handles two cases:
	//   cordon --mcp   → launch MCP server
	//   cordon         → print help
	// Subcommands are unaffected by --mcp; they handle their own RunE.
	RunE: func(cmd *cobra.Command, args []string) error {
		if mcpMode {
			return runMCPServer()
		}
		return cmd.Help()
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if flags.JSON || mcpMode {
			return
		}
		if cmd.Name() == "hook" {
			return
		}
		updatecheck.MaybeRun(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), buildinfo.Version)
	},
}

// Execute is the entry point called from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// --json: structured output consumed by the IDE extension.
	// Stored in flags.JSON so subpackages can read it without circular imports.
	rootCmd.PersistentFlags().BoolVar(&flags.JSON, "json", false, "Output as JSON")

	// --mcp: run as a stdio MCP server. Meaningful only on root, not subcommands.
	rootCmd.Flags().BoolVar(&mcpMode, "mcp", false, "Run as a stdio MCP server")

	// --agent: identifies which agent platform is invoking the hook.
	hookCmd.Flags().StringVar(&hookAgent, "agent", "", "Agent platform identifier (e.g. claude-code, cursor)")

	rootCmd.AddCommand(
		initCmd,
		loginCmd,
		logoutCmd,
		statusCmd,
		syncCmd,
		hookCmd,
		logCmd,
		uninstallCmd,
		versionCmd,
		auth.Cmd,
		file.Cmd,
		pass.Cmd,
		command.Cmd,
		sessionsCmd,
	)
}

// runMCPServer starts the stdio MCP server and blocks until the client disconnects.
func runMCPServer() error {
	return mcpserver.Run(context.Background())
}
