package main

import (
	"application-use/internal/cli"
	"golang.design/x/hotkey/mainthread"
)

func main() {
	// macOS requires window/UI events to run on the main thread
	mainthread.Init(func() {
		cli.Execute()
	})
}
