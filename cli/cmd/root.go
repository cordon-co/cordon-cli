// Package cmd implements the cordon CLI command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/cordon-co/cordon/cmd/pass"
	"github.com/cordon-co/cordon/cmd/zone"
	"github.com/cordon-co/cordon/internal/flags"
	"github.com/spf13/cobra"
)

var mcpMode bool

var rootCmd = &cobra.Command{
	Use:   "cordon",
	Short: "Team-wide access policies for AI coding agents",
	Long: `Cordon enforces file-level write restrictions on AI agents. Zones define protected files, folders or patterns; passes grant
temporary agent access; the audit log captures every enforcement decision.`,
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

	rootCmd.AddCommand(
		initCmd,
		loginCmd,
		logoutCmd,
		statusCmd,
		syncCmd,
		hookCmd,
		logCmd,
		removeCmd,
		versionCmd,
		zone.Cmd,
		pass.Cmd,
	)
}

// runMCPServer is a stub for the stdio MCP server.
// Implementation will use github.com/mark3labs/mcp-go.
func runMCPServer() error {
	if flags.JSON {
		fmt.Println(`{"error":"not implemented"}`)
		return nil
	}
	fmt.Fprintln(os.Stderr, "not implemented")
	return nil
}
