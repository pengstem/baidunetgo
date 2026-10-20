package app

import (
	"embed"
	"log"
	"strings"

	"github.com/egoist/mygo/ui"
)

// IBM Plex Sans for words and IBM Plex Mono for figures (OFL, see
// fonts/OFL.txt). Chinese falls back to the system's CJK face.
//
//go:embed fonts/*.otf
var fontFS embed.FS

const (
	fontSans = "IBM Plex Sans"
	fontMono = "IBM Plex Mono"
)

func init() {
	files, _ := fontFS.ReadDir("fonts")
	for _, f := range files {
		data, err := fontFS.ReadFile("fonts/" + f.Name())
		if err != nil {
			log.Printf("font %s: %v", f.Name(), err)
			continue
		}
		family := fontSans
		if strings.HasPrefix(f.Name(), "IBMPlexMono") {
			family = fontMono
		}
		if err := ui.RegisterFont(data, family); err != nil {
			log.Printf("font %s: %v", f.Name(), err)
		}
	}
}
