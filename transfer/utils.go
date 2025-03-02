// transfer/utils.go
package transfer

import (
	"io"
	"sync"

	"github.com/juju/ratelimit"
)

var (
	rateLimiterCache     *ratelimit.Bucket
	lastSpeedLimit       int
	rateLimiterCacheLock sync.Mutex
)

func WrapRateLimitedWriter(w io.Writer, speedLimit int) io.Writer {
	if speedLimit <= 0 {
		return w
	}

	rateLimiterCacheLock.Lock()
	defer rateLimiterCacheLock.Unlock()

	if rateLimiterCache != nil && lastSpeedLimit == speedLimit {
		return ratelimit.Writer(w, rateLimiterCache)
	}

	rateLimiterCache = ratelimit.NewBucketWithRate(float64(speedLimit), int64(speedLimit))
	lastSpeedLimit = speedLimit

	return ratelimit.Writer(w, rateLimiterCache)
}
