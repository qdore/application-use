package cli

import (
	"application-use/internal/appuse"
	"application-use/internal/cgo/macos/appuse_bridge"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(clickCmd)
	rootCmd.AddCommand(fillCmd)
	rootCmd.AddCommand(openCmd)
	screenshotCmd.Flags().StringP("frame", "f", "", "Target frame (x,y,w,h) for the screenshot")

	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(closeCmd)
	rootCmd.AddCommand(screenshotCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(sendkeyCmd)
	rootCmd.AddCommand(scrollCmd)
	snapshotCmd.Flags().Bool("debug", false, "Enable debug mode (saves debug.png with separators)")
	rootCmd.AddCommand(upgradeCmd)

	clickCmd.Flags().BoolP("right", "r", false, "Perform a right-click")
	clickCmd.Flags().BoolP("double", "d", false, "Perform a double-click")
}

var clickCmd = &cobra.Command{
	Use:   "click <ref>",
	Short: "Click an element by its hint letters (e.g. 'JK', also support --right/--double)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		right, _ := cmd.Flags().GetBool("right")
		double, _ := cmd.Flags().GetBool("double")

		mode := appuse.ClickLeft
		if right {
			mode = appuse.ClickRight
		} else if double {
			mode = appuse.ClickDouble
		}

		if err := appuse.Click(args[0], mode, appName); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var fillCmd = &cobra.Command{
	Use:     "fill [hint] <text>",
	Aliases: []string{"type"},
	Short:   "Fill text into an element by its hint or the focused element (uses clipboard paste)",
	Args:    cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		hint := ""
		text := ""
		if len(args) == 2 {
			hint = args[0]
			text = args[1]
		} else {
			text = args[0]
		}

		if err := appuse.Type(hint, text, appName); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Take a snapshot of current window and show element hints. (+) means newly added elements (*) means ocr element.",
	Run: func(cmd *cobra.Command, args []string) {
		debug, _ := cmd.Flags().GetBool("debug")
		jsonOutput := appuse.TakeSnapshot()

		areas := appuse.PrintSnapshot(jsonOutput, debug)
		appuse.SaveCache(jsonOutput, areas)

		// Keep the CLI process alive for 1 second so the window stays visible.
		time.Sleep(1 * time.Second)


		appuse.ClearSnapshot()
	},
}

var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open application specified by --appName",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Permission checks
		var hasError bool
		if !appuse_bridge.CheckAccessibilityPermission(false) {
			fmt.Println("Error: Accessibility permission denied.")
			fmt.Println("Please grant accessibility permissions to 'application-use' in System Settings > Privacy & Security > Accessibility.")
			hasError = true
		}
		if !appuse_bridge.CheckScreenRecordingPermission() {
			fmt.Println("Error: Screen Recording permission denied.")
			fmt.Println("Please grant screen recording permissions to 'application-use' in System Settings > Privacy & Security > Screen Recording.")
			hasError = true
		}
		if hasError {
			os.Exit(1)
		}
		appuse.AutoCheckUpdate()

		if appName == "" {
			fmt.Println("Error: must provide an application name via --appName")
			os.Exit(1)
		}
		if err := appuse.Open(appName); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var closeCmd = &cobra.Command{
	Use:   "close",
	Short: "Close application specified by --appName",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if appName == "" {
			fmt.Println("Error: must provide an application name via --appName")
			os.Exit(1)
		}
		if err := appuse.Close(appName); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}
var screenshotCmd = &cobra.Command{
	Use:   "screenshot [path]",
	Short: "Take a screenshot of the current window or a specific frame",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		frame, _ := cmd.Flags().GetString("frame")

		if err := appuse.Screenshot(path, frame, appName); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search for installed applications",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := ""
		if len(args) > 0 {
			query = args[0]
		} else if appName != "" {
			query = appName
		}
		apps, err := appuse.Search(query)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Found %d applications:(appName \t appPath)\n", len(apps))
		for _, app := range apps {
			fmt.Printf("  - %-20s (%s) %s\n", app.FileName, app.Name, app.Path)
		}
	},
}

var sendkeyCmd = &cobra.Command{
	Use:   "sendkey <key>",
	Short: "Send a key or combination (e.g., cmd+v, enter, esc) to the target app",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := appuse.SendKey(args[0], appName); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var scrollCmd = &cobra.Command{
	Use:   "scroll <area> <direction> [distance]",
	Short: "Scroll in a specified area (e.g., Main, Area 1) and direction (up/down/left/right)",
	Args:  cobra.RangeArgs(2, 3),
	Run: func(cmd *cobra.Command, args []string) {
		area := args[0]
		direction := args[1]
		distance := 0
		if len(args) == 3 {
			fmt.Sscanf(args[2], "%d", &distance)
		}

		if err := appuse.Scroll(area, direction, distance, appName); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Check for updates and automatically upgrade if available",
	Run: func(cmd *cobra.Command, args []string) {
		latest, err := appuse.CheckUpdate()
		if err != nil {
			fmt.Printf("Notice: %v\n", err)
			return
		}

		if latest != "" {
			fmt.Printf("\n🚀 A new version of application-use is available: %s (current: %s)\n", latest, appuse.Version)
			if err := appuse.PerformUpgrade(latest); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Printf("application-use is already at the latest version (%s).\n", appuse.Version)
		}
	},
}
