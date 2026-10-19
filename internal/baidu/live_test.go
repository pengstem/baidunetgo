//go:build live

package baidu

import (
	"context"
	"testing"
	"time"
)

// go test -tags live ./internal/baidu -run Live -v
func TestLiveQRCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	q, err := NewQRLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Image) < 100 || string(q.Image[1:4]) != "PNG" {
		t.Fatalf("not a PNG: %d bytes %q", len(q.Image), q.Image[:min(16, len(q.Image))])
	}
	st, err := q.Poll(ctx)
	t.Logf("sign %s, image %d bytes, state %v, err %v", q.sign, len(q.Image), st, err)
}
