package app

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// Icons are Lucide's (ISC), embedded and parsed on first use.
//
//go:embed icons/*.svg
var iconFS embed.FS

var icons = map[string]*ui.SVG{}

// icon returns the Lucide icon of that name. It runs on the UI thread only.
func icon(name string) *ui.SVG {
	if s, ok := icons[name]; ok {
		return s
	}
	data, err := iconFS.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic("app: no icon " + name)
	}
	s := ui.MustParseSVG(data)
	icons[name] = s
	return s
}
