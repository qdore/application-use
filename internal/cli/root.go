package cli

import (
	"fmt"
	"os"

	"application-use/internal/appuse"
	"time"

	"github.com/spf13/cobra"
)

var appName string

var rootCmd = &cobra.Command{
	Use:   "application-use",
	Short: "application-use is a headless macOS automation agent",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip activation for search and help commands
		if cmd.Name() == "search" || cmd.Name() == "help" || cmd.Name() == "application-use" {
			return nil
		}

		if appName == "" {
			// Resolve from cache if not provided
			cachedName, _ := appuse.GetMostRecentApp()
			if cachedName != "" {
				appName = cachedName
			}
		}

		if appName != "" {
			return appuse.Activate(appName)
		}
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// Only run for specific interactive commands
		targetCmds := map[string]bool{
			"click":   true,
			"scroll":  true,
			"fill":    true,
			"open":    true,
			"sendkey": true,
		}
		if !targetCmds[cmd.Name()] {
			return
		}

		// Wait for UI to stabilize after action
		time.Sleep(500 * time.Millisecond)
		fmt.Printf("========= Now current snapshot after %s ======\n", cmd.Name())

		jsonOutput := appuse.TakeSnapshot()
		areas := appuse.PrintSnapshot(jsonOutput, false)
		appuse.SaveCache(jsonOutput, areas)

		// Keep the CLI process alive for 1 second so the window stays visible.
		time.Sleep(1 * time.Second)
		appuse.ClearSnapshot()
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&appName, "appName", "a", "", "Target application name")
}

// Execute is the main entry point to the CLI
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
