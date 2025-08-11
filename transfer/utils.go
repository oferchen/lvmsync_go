// Package transfer contains helpers for the data path.
// WrapRateLimitedWriter returns a writer capped to speedLimit bytes per second.
//
// Example:
//
//	w := WrapRateLimitedWriter(dst, 1<<20) // 1 MiB/s
//	w.Write(p)
package transfer

import (
	"io"
	"sync"

	"lvmsync_go/internal/limiter"
)

// rateLimitedWriter wraps an io.Writer with a token bucket limiter.
type rateLimitedWriter struct {
	w   io.Writer
	tb  limiter.Limiter
	max int
}

func (rlw *rateLimitedWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		chunk := len(p)
		if chunk > rlw.max {
			chunk = rlw.max
		}
		rlw.tb.Allow(chunk)
		n, err := rlw.w.Write(p[:chunk])
		written += n
		if err != nil {
			return written, err
		}
		p = p[n:]
	}
	return written, nil
}

var (
	rateLimiterCache     limiter.Limiter
	lastSpeedLimit       int
	rateLimiterCacheLock sync.Mutex
)

// WrapRateLimitedWriter returns an io.Writer that limits throughput to
// speedLimit bytes per second. The limiter instance is cached for reuse.
func WrapRateLimitedWriter(w io.Writer, speedLimit int) io.Writer {
	if speedLimit <= 0 {
		return w
	}

	rateLimiterCacheLock.Lock()
	if rateLimiterCache == nil || lastSpeedLimit != speedLimit {
		rateLimiterCache = limiter.New(speedLimit, speedLimit, nil)
		lastSpeedLimit = speedLimit
	}
	rlw := &rateLimitedWriter{w: w, tb: rateLimiterCache, max: speedLimit}
	rateLimiterCacheLock.Unlock()
	return rlw
}
