package cmd

import (
	"fmt"

	"github.com/cordon-co/cordon-cli/cli/internal/buildinfo"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the cordon version",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flags.JSON {
			fmt.Printf(`{"version":%q}`+"\n", buildinfo.Version)
			return nil
		}
		fmt.Println(buildinfo.Version)
		return nil
	},
}
