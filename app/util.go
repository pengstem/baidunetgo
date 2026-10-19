package app

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
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

// humanSpeed writes bytes per second.
func humanSpeed(bps float64) string {
	if bps <= 0 {
		return "—"
	}
	return humanSize(int64(bps)) + "/s"
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
