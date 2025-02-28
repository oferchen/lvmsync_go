// transfer/utils.go
package transfer

import (
	"io"

	"github.com/juju/ratelimit"
)

func WrapRateLimitedWriter(w io.Writer, speedLimit int) io.Writer {
	if speedLimit > 0 {
		bucket := ratelimit.NewBucketWithRate(float64(speedLimit), int64(speedLimit))
		return ratelimit.Writer(w, bucket)
	}
	return w
}
