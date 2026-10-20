package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"github.com/pengstem/baidunetgo/internal/tune"
)

// settings are the user's choices, kept as JSON in the app's data
// directory.
type settings struct {
	MPV         string `json:"mpv,omitempty"`          // mpv's path, found on its own when empty
	DownloadDir string `json:"download_dir,omitempty"` // where downloads go, Downloads when empty
	Threads     int    `json:"threads,omitempty"`      // connections per download
	Parallel    int    `json:"parallel,omitempty"`     // downloads at once
	PreviewMB   int    `json:"preview_mb,omitempty"`   // largest picture or text previewed, in MB
	// Manual keeps Threads and Parallel as set; else they follow the
	// membership and the last measurement.
	Manual bool `json:"manual_transfer,omitempty"`
	// Tune is the last measurement of download speeds, and the
	// membership it was made with.
	Tune *tuneRecord `json:"tune,omitempty"`
	// SkipImport stops signing in with another client's account by
	// itself, once the user signed out.
	SkipImport bool `json:"skip_pcs_import,omitempty"`
}

type tuneRecord struct {
	VIP    int          `json:"vip"`
	Result *tune.Result `json:"result"`
}

func settingsPath() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "settings.json")
}

func loadSettings() settings {
	var s settings
	if p := settingsPath(); p != "" {
		if data, err := os.ReadFile(p); err == nil {
			json.Unmarshal(data, &s)
		}
	}
	return s
}

func (s settings) save() error {
	p := settingsPath()
	if p == "" {
		return nil
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func (s settings) threads() int {
	if s.Threads <= 0 {
		return 4
	}
	return s.Threads
}

func (s settings) parallel() int {
	if s.Parallel <= 0 {
		return 2
	}
	return s.Parallel
}

func (s settings) downloadDir() string {
	if s.DownloadDir != "" {
		return s.DownloadDir
	}
	if d, err := mygo.App.Path(mygo.PathDownloads); err == nil {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}
