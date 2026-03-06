package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "abacus",
	Short: "Living application graph for Gherkin-driven BDD",
	Long:  "Abacus models your application's API surface, entities, pages, and user actions. It bridges Gherkin specs to implementation through a persistent, cumulative graph.",
	Version: Version,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.SetVersionTemplate("abacus {{.Version}}\n")
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("abacus", Version)
	},
}
