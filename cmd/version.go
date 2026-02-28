package cmd

import (
	"fmt"

	"github.com/badimirzai/architon-cli/internal/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show the installed CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version.Line())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
