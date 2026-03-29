package cli

import (
	"application-use/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(daemonCmd)
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Starts the persistent background automation daemon",
	Run: func(cmd *cobra.Command, args []string) {
		daemon.Start()
	},
}
