package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "pwctl",
	Short: "Check if the server is live",
	Long:  `A command line tool to check if the server is live`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Please use a subcommand")
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
