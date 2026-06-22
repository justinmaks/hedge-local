package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the hcli version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "hcli %s\n", Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
