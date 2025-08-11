// Package limiter tests verify token bucket accuracy.
//
// Example:
//
//	tb := New(1024, 1024, clk)
//	tb.Allow(512)
package limiter

import (
	"math"
	"testing"
	"time"
)

type stubClock struct{ now time.Time }

func (s *stubClock) Now() time.Time        { return s.now }
func (s *stubClock) Sleep(d time.Duration) { s.now = s.now.Add(d) }

func TestTokenBucketAccuracy(t *testing.T) {
	clk := &stubClock{now: time.Unix(0, 0)}
	rate := 1024
	tb := New(rate, rate, clk)
	tb.Allow(rate)
	start := clk.Now()
	tb.Allow(rate)
	elapsed := clk.Now().Sub(start)
	expected := time.Second
	if math.Abs(float64(elapsed-expected)) > float64(expected)*0.03 {
		t.Fatalf("elapsed %v outside ±3%% of %v", elapsed, expected)
	}
}
