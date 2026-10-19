package main

import (
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
)

// captureOnStart writes a PNG of the window to $BAIDUNETGO_CAPTURE a few
// seconds after it opens and quits, for looking at the interface from
// scripts. $BAIDUNETGO_CAPTURE_AFTER sets the wait (a Go duration).
func captureOnStart(win *mygo.Window) {
	path := os.Getenv("BAIDUNETGO_CAPTURE")
	if path == "" {
		return
	}
	wait := 3 * time.Second
	if d, err := time.ParseDuration(os.Getenv("BAIDUNETGO_CAPTURE_AFTER")); err == nil {
		wait = d
	}
	time.AfterFunc(wait, func() {
		png, err := win.CapturePage()
		if err != nil {
			log.Printf("capture: %v", err)
		} else if err := os.WriteFile(path, png, 0o644); err != nil {
			log.Printf("capture: %v", err)
		}
		mygo.App.Quit()
	})
}
