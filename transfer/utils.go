// transfer/utils.go
package transfer

import (
	"context"
	"io"
	"sync"

	"golang.org/x/time/rate"
)

// rateLimitedWriter wraps an io.Writer and throttles write throughput to a
// specified number of bytes per second using a shared rate limiter.
type rateLimitedWriter struct {
	w       io.Writer
	limiter *rate.Limiter
}

func (rlw *rateLimitedWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		chunk := len(p)
		if burst := rlw.limiter.Burst(); chunk > burst {
			chunk = burst
		}
		if err := rlw.limiter.WaitN(context.Background(), chunk); err != nil {
			return written, err
		}
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
	rateLimiterCache     *rate.Limiter
	lastSpeedLimit       int
	rateLimiterCacheLock sync.Mutex
)

// WrapRateLimitedWriter returns an io.Writer that limits the write throughput
// to the specified speedLimit in bytes per second. The limiter instance is
// cached to avoid unnecessary allocations when the limit remains unchanged.
func WrapRateLimitedWriter(w io.Writer, speedLimit int) io.Writer {
	if speedLimit <= 0 {
		return w
	}

	rateLimiterCacheLock.Lock()
	defer rateLimiterCacheLock.Unlock()

	if rateLimiterCache == nil || lastSpeedLimit != speedLimit {
		rateLimiterCache = rate.NewLimiter(rate.Limit(speedLimit), speedLimit)
		lastSpeedLimit = speedLimit
	}

	return &rateLimitedWriter{w: w, limiter: rateLimiterCache}
}
