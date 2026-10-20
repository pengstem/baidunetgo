package app

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// sinTurn returns the sine of a fraction of a turn.
func sinTurn(x float32) float64 { return math.Sin(2 * math.Pi * float64(x)) }

// readAll reads r to its end, failing past limit bytes.
func readAll(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("too large")
	}
	return data, nil
}

// humanSize writes a size in bytes as people read it.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	for _, u := range []string{"KB", "MB", "GB", "TB"} {
		f /= unit
		if f < unit {
			if f < 10 {
				return fmt.Sprintf("%.1f %s", f, u)
			}
			return fmt.Sprintf("%.0f %s", f, u)
		}
	}
	return fmt.Sprintf("%.0f PB", f/unit)
}

// humanTime writes a moment relative to now when recent.
func humanTime(unix int64, now time.Time) string {
	t := time.Unix(unix, 0)
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour && t.Day() == now.Day():
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	case t.Year() == now.Year():
		return t.Format("1月2日 15:04")
	}
	return t.Format("2006/01/02")
}

// stampTime writes when a file changed, as a list column shows it: the
// time today, the day this year, the date before.
func stampTime(unix int64, now time.Time) string {
	t := time.Unix(unix, 0)
	switch {
	case t.Year() == now.Year() && t.YearDay() == now.YearDay():
		return "今天 " + t.Format("15:04")
	case t.Year() == now.Year():
		return t.Format("01-02 15:04")
	}
	return t.Format("2006-01-02")
}

// shortSize writes a size in the fewest characters: 734G, 1.5T.
func shortSize(n int64) string {
	s := strings.ReplaceAll(humanSize(n), " ", "")
	if strings.HasSuffix(s, "B") && len(s) > 2 && s[len(s)-2] >= 'A' {
		s = s[:len(s)-1]
	}
	return s
}

// tildePath writes a local path with the home folder as ~.
func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// humanSpeed writes bytes per second.
func humanSpeed(bps float64) string {
	if bps <= 0 {
		return "—"
	}
	return humanSize(int64(bps)) + "/s"
}

// indexFold returns where q is in s, case aside, -1 if nowhere.
func indexFold(s, q string) int {
	i := strings.Index(strings.ToLower(s), strings.ToLower(q))
	if i < 0 || i+len(q) > len(s) {
		return -1
	}
	return i
}

func itoa(n int) string { return strconv.Itoa(n) }

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func clamp(x, lo, hi float32) float32 { return max(lo, min(x, hi)) }
