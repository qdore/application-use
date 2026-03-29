package daemon

import (
	"application-use/internal/appuse"
	"fmt"
	"log"
	"time"

	"golang.design/x/hotkey"
)

// Start initiates the persistent daemon, handling background global elements like hotkeys.
func Start() {
	hk := hotkey.New([]hotkey.Modifier{hotkey.ModCmd, hotkey.ModShift}, hotkey.KeySpace)
	err := hk.Register()
	if err != nil {
		log.Fatalf("hotkey: failed to register hotkey: %v", err)
	}

	fmt.Println("application-use daemon started.")
	fmt.Println("Press Cmd+Shift+Space to trigger snapshot. Press Ctrl+C to exit.")

	for {
		<-hk.Keydown()

		// Local loop for the active snapshot "session"
		for {
			fmt.Printf("[%s] Triggering snapshot...\n", time.Now().Format("15:04:05"))
			jsonOutput := appuse.TakeSnapshot()
			appuse.PrintSnapshot(jsonOutput, false)
			fmt.Println("  (Press ESC to cancel, or Cmd+Shift+Space to refresh)")

			// Register ESC only while snapshot is active
			esc := hotkey.New([]hotkey.Modifier{}, hotkey.KeyEscape)
			if err := esc.Register(); err != nil {
				fmt.Printf("Warning: failed to register ESC hotkey: %v\n", err)
				// If we can't register ESC, we just clear and break to avoid being stuck
				time.Sleep(2 * time.Second)
				appuse.ClearSnapshot()
				break
			}

			cancelled := false
			refreshed := false

			select {
			case <-esc.Keydown():
				cancelled = true
			case <-hk.Keydown():
				refreshed = true
			}

			esc.Unregister()
			appuse.ClearSnapshot()

			if cancelled {
				fmt.Println("  Snapshot dismissed.")
				break
			}
			if refreshed {
				fmt.Println("  Refreshing snapshot...")
				continue
			}
		}
	}
}
