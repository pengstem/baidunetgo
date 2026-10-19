// Package player plays netdisk media in mpv, which streams it from its
// link with the headers the netdisk asks for: nothing touches the disk.
package player

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ErrNoMPV is what Find returns when mpv is not installed.
var ErrNoMPV = errors.New("没有找到 mpv，请安装 mpv 或在设置中指定它的位置")

// Find returns the path of mpv: custom when set, else from PATH or where
// the platform's installers put it.
func Find(custom string) (string, error) {
	if custom != "" {
		if _, err := os.Stat(custom); err == nil {
			return custom, nil
		}
		return "", ErrNoMPV
	}
	if p, err := exec.LookPath("mpv"); err == nil {
		return p, nil
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/Applications/mpv.app/Contents/MacOS/mpv",
			"/opt/homebrew/bin/mpv",
			"/usr/local/bin/mpv",
		}
	case "windows":
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			if dir := os.Getenv(env); dir != "" {
				candidates = append(candidates, filepath.Join(dir, "mpv", "mpv.exe"))
			}
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, "scoop", "shims", "mpv.exe"))
		}
	default:
		candidates = []string{"/usr/bin/mpv", "/usr/local/bin/mpv", "/snap/bin/mpv", "/var/lib/flatpak/exports/bin/io.mpv.Mpv"}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", ErrNoMPV
}

// Args returns mpv's arguments to play url with the headers it needs,
// titled title.
func Args(url, title string, h http.Header) []string {
	args := []string{
		"--force-window=immediate",
		"--keep-open=no",
		"--force-media-title=" + title,
		"--title=" + title + " — BaiduNetGo",
	}
	if ua := h.Get("User-Agent"); ua != "" {
		args = append(args, "--user-agent="+ua)
	}
	// One header at a time: the list option splits on commas.
	for name, values := range h {
		if name == "User-Agent" {
			continue
		}
		for _, v := range values {
			args = append(args, "--http-header-fields-append="+name+": "+v)
		}
	}
	return append(args, "--", url)
}

// Play starts mpv playing url. It returns once mpv has started; mpv lives
// on after the app quits.
func Play(mpv, url, title string, h http.Header) error {
	cmd := exec.Command(mpv, Args(url, title, h)...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
