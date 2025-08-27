// Package transfer tests rate-limited writer behavior.
//
// Example:
//
//	w := &rateLimitedWriter{w: dst, tb: limiter.New(1024, 1024, clk), max: 1024}
//	w.Write(p)
package transfer

import (
	"bytes"
	"io"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/oferchen/lvmsync_go/internal/limiter"
)

type stubClock struct {
	mu  sync.Mutex
	now time.Time
}

func (s *stubClock) Now() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.now
}

func (s *stubClock) Sleep(d time.Duration) {
	s.mu.Lock()
	s.now = s.now.Add(d)
	s.mu.Unlock()
}

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

func TestRateLimitedWritersIndependent(t *testing.T) {
	clk1 := &stubClock{now: time.Unix(0, 0)}
	clk2 := &stubClock{now: time.Unix(0, 0)}
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}
	w1 := &rateLimitedWriter{w: buf1, tb: limiter.New(1024, 1024, clk1), max: 1024}
	w2 := &rateLimitedWriter{w: buf2, tb: limiter.New(2048, 2048, clk2), max: 2048}
	data := make([]byte, 4096)

	var wg sync.WaitGroup
	wg.Add(2)
	var d1, d2 time.Duration

	go func() {
		start := clk1.Now()
		if _, err := w1.Write(data); err != nil {
			t.Errorf("w1 write: %v", err)
		}
		d1 = clk1.Now().Sub(start)
		wg.Done()
	}()

	go func() {
		start := clk2.Now()
		if _, err := w2.Write(data); err != nil {
			t.Errorf("w2 write: %v", err)
		}
		d2 = clk2.Now().Sub(start)
		wg.Done()
	}()
	wg.Wait()

	if math.Abs(float64(d1-3*time.Second)) > float64(3*time.Second)*0.03 {
		t.Fatalf("writer1 elapsed %v outside ±3%% of %v", d1, 3*time.Second)
	}
	if math.Abs(float64(d2-time.Second)) > float64(time.Second)*0.03 {
		t.Fatalf("writer2 elapsed %v outside ±3%% of %v", d2, time.Second)
	}
}

func BenchmarkRateLimitedWritersIndependent(b *testing.B) {
	for i := 0; i < b.N; i++ {
		clk := &stubClock{now: time.Unix(0, 0)}
		w1 := &rateLimitedWriter{w: io.Discard, tb: limiter.New(1024, 1024, clk), max: 1024}
		w2 := &rateLimitedWriter{w: io.Discard, tb: limiter.New(2048, 2048, clk), max: 2048}
		data := make([]byte, 4096)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { w1.Write(data); wg.Done() }()
		go func() { w2.Write(data); wg.Done() }()
		wg.Wait()
	}
}
