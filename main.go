// BaiduNetGo is a desktop client for Baidu Netdisk, drawn natively with
// MyGo and talking to the netdisk through BaiduPCS-Go.
package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans" // MyGo's own strings in Chinese
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/app"
)

func main() {
	// The interface is written in Chinese: MyGo's own controls and menus
	// speak it too.
	mygo.App.SetLanguages("zh-Hans")
	// One name however it is launched, so that go run, mygo dev and the
	// packaged app share the session and settings.
	mygo.App.SetName("BaiduNetGo")
	if selfTest() {
		if err := mygo.App.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}
	// One window at a time: two would run the same transfers into the
	// same files. A later launch brings the first forward instead. Runs
	// against the pretend netdisk, or capturing for scripts, may go
	// beside it.
	alone := os.Getenv("BAIDUNETGO_FAKE") != "" || os.Getenv("BAIDUNETGO_CAPTURE") != ""
	if !alone && !mygo.App.RequestSingleInstanceLock() {
		return
	}
	a := app.New()
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.Shutdown() })
	// Before Run, to hear of a click that launched the app.
	mygo.App.OnNotificationClick(a.Notification)
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "BaiduNetGo",
			Width:     1180,
			Height:    760,
			MinWidth:  780,
			MinHeight: 540,
			StateKey:  "main",
			// A native window: the traffic lights inset over the sidebar,
			// which shows the window's translucent material (macOS,
			// Windows 11) where it draws no background.
			// macOS takes the traffic lights' place rather than the
			// height: 16 + 2×18 makes them a 52 DIP bar, centered in it.
			TitleBarStyle:        mygo.TitleBarHiddenInset,
			TrafficLightPosition: &mygo.Point{X: 20, Y: 18},
			TitleBarHeight:       52,
			Vibrancy:             mygo.VibrancySidebar,
			Content:              ui.View(a.View),
		})
		a.Attach(win)
		mygo.App.OnSecondInstance(func([]string, string) {
			if win.IsMinimized() {
				win.Restore()
			}
			win.Show()
		})
		captureOnStart(win)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
