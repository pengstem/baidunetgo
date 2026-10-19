// BaiduNetGo is a desktop client for Baidu Netdisk, drawn natively with
// MyGo and talking to the netdisk through BaiduPCS-Go.
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/app"
)

func main() {
	a := app.New()
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "BaiduNetGo",
			Width:           1180,
			Height:          760,
			MinWidth:        780,
			MinHeight:       540,
			StateKey:        "main",
			TitleBarStyle:   mygo.TitleBarHidden,
			BackgroundColor: "light-dark(#fbfbfc, #0f1013)",
			Content:         ui.View(a.View),
		})
		a.Attach(win)
		captureOnStart(win)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
