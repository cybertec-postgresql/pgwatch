package cmd

import (
	"fmt"

	"github.com/cybertec-postgresql/pgwatch3/pwctl"
	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the server status",
	Long:  `Check the server status`,
	Run: func(cmd *cobra.Command, args []string) {
		serverAddress, _ := cmd.Flags().GetString("c")
		if pwctl.IsServerLive(serverAddress) {
			fmt.Println("pwctl is running")
		} else {
			fmt.Println("pwctl is not running")
		}
	},
}

func init() {
	statusCmd.Flags().StringP("c", "c", "", "server address (required)")
	statusCmd.MarkFlagRequired("c")
	rootCmd.AddCommand(statusCmd)
}
