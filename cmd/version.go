package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/71g3pf4c3/charss/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ver, commit, date := version.Info()
		fmt.Printf("charss %s\ncommit %s\nbuilt %s\n", ver, commit, date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
