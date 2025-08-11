package limiter

import (
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (f *fakeClock) Now() time.Time        { return f.now }
func (f *fakeClock) Sleep(d time.Duration) { f.now = f.now.Add(d) }

func TestAllow(t *testing.T) {
	fc := &fakeClock{now: time.Unix(0, 0)}
	tb := New(1000, 1000, fc)
	tb.Allow(1000)
	if fc.now != time.Unix(0, 0) {
		t.Fatalf("unexpected advance")
	}
	tb.Allow(1000)
	if fc.now.Sub(time.Unix(0, 0)) < time.Second {
		t.Fatalf("expected at least 1s wait")
	}
}
