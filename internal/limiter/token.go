// Package limiter provides a token-bucket rate limiter used to cap outgoing
// bandwidth based on post-compression bytes.
package limiter

import (
	"sync"
	"time"
)

// Limiter throttles data transfers.
type Limiter interface {
	Allow(n int)
}

// Clock abstracts time for testability.
type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// realClock implements Clock using the system clock.
type realClock struct{}

func (realClock) Now() time.Time        { return time.Now() }
func (realClock) Sleep(d time.Duration) { time.Sleep(d) }

// TokenBucket implements Limiter.
type TokenBucket struct {
	rate   float64
	burst  float64
	mu     sync.Mutex
	tokens float64
	last   time.Time
	clk    Clock
}

// New creates a token bucket with rate bytes per second and burst capacity.
func New(rate, burst int, clk Clock) *TokenBucket {
	if clk == nil {
		clk = realClock{}
	}
	return &TokenBucket{rate: float64(rate), burst: float64(burst), tokens: float64(burst), last: clk.Now(), clk: clk}
}

// Allow waits until n tokens are available.
func (t *TokenBucket) Allow(n int) {
	if t.rate <= 0 {
		return
	}
	need := float64(n)
	for {
		t.mu.Lock()
		now := t.clk.Now()
		elapsed := now.Sub(t.last).Seconds()
		if elapsed > 0 {
			t.tokens += elapsed * t.rate
			if t.tokens > t.burst {
				t.tokens = t.burst
			}
			t.last = now
		}
		if t.tokens >= need {
			t.tokens -= need
			t.mu.Unlock()
			return
		}
		wait := (need - t.tokens) / t.rate
		t.mu.Unlock()
		t.clk.Sleep(time.Duration(wait * float64(time.Second)))
	}
}
