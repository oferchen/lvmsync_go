// Package transfer tests rate-limited writer behavior.
//
// Example:
//
//	w := &rateLimitedWriter{w: dst, tb: limiter.New(1024, 1024, clk), max: 1024}
//	w.Write(p)
package transfer

import (
	"bytes"
	"math"
	"testing"
	"time"

	"lvmsync_go/internal/limiter"
)

type stubClock struct{ now time.Time }

func (s *stubClock) Now() time.Time        { return s.now }
func (s *stubClock) Sleep(d time.Duration) { s.now = s.now.Add(d) }

func TestRateLimitedWriterAccuracy(t *testing.T) {
	clk := &stubClock{now: time.Unix(0, 0)}
	l := limiter.New(1024, 1024, clk)
	buf := &bytes.Buffer{}
	w := &rateLimitedWriter{w: buf, tb: l, max: 1024}
	data := make([]byte, 2048)
	start := clk.Now()
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	elapsed := clk.Now().Sub(start)
	expected := time.Second
	if math.Abs(float64(elapsed-expected)) > float64(expected)*0.03 {
		t.Fatalf("elapsed %v outside ±3%% of %v", elapsed, expected)
	}
	if buf.Len() != len(data) {
		t.Fatalf("wrote %d bytes want %d", buf.Len(), len(data))
	}
}
