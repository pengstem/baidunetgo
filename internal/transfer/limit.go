package transfer

import (
	"context"
	"sync"
	"time"
)

// limiter lets bytes through at most at a rate, shared by the transfers
// going one way.
type limiter struct {
	mu   sync.Mutex
	rate int64     // bytes a second, 0 for no limit
	next time.Time // when the bytes let through so far are due
}

func (l *limiter) set(rate int64) {
	l.mu.Lock()
	l.rate = max(rate, 0)
	l.mu.Unlock()
}

// wait holds n bytes back until the rate lets them through.
func (l *limiter) wait(ctx context.Context, n int64) error {
	l.mu.Lock()
	if l.rate <= 0 || n <= 0 {
		l.mu.Unlock()
		return nil
	}
	now := time.Now()
	if l.next.Before(now) {
		l.next = now // no credit for the time it went unused
	}
	l.next = l.next.Add(time.Duration(n) * time.Second / time.Duration(l.rate))
	d := time.Until(l.next)
	l.mu.Unlock()
	if d <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
