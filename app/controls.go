package app

import (
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// controlsRoom returns the room the window buttons take at the end of the
// title bar. On Linux MyGo reports the width of a whole GTK header bar,
// whose empty title area makes it some 90 DIP wider than its buttons, so
// there it is the buttons alone, counted from GNOME's layout: Adwaita's
// are 34 DIP wide, 6 apart and 6 from the edge. Elsewhere, or when the
// layout cannot be read, it is what MyGo reports.
func controlsRoom(bar ui.TitleBar) float32 {
	if runtime.GOOS != "linux" || bar.Right <= 0 {
		return bar.Right
	}
	n := endButtons()
	if n <= 0 {
		return bar.Right
	}
	return min(bar.Right, float32(n)*40)
}

// endButtons counts the window buttons GNOME puts at the end of a title
// bar, or returns -1 when it cannot tell.
var endButtons = sync.OnceValue(func() int {
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.wm.preferences", "button-layout").Output()
	if err != nil {
		return -1
	}
	_, end, ok := strings.Cut(strings.Trim(strings.TrimSpace(string(out)), "'"), ":")
	if !ok {
		return -1
	}
	n := 0
	for b := range strings.SplitSeq(end, ",") {
		switch strings.TrimSpace(b) {
		case "minimize", "maximize", "close":
			n++
		}
	}
	return n
})
