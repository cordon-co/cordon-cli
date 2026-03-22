package cmd

import (
	"fmt"

	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X github.com/cordon-co/cordon-cli/cmd.Version=<tag>".
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the cordon version",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flags.JSON {
			fmt.Printf(`{"version":%q}`+"\n", Version)
			return nil
		}
		fmt.Println(Version)
		return nil
	},
}
