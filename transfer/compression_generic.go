//go:build !amd64 && !386 && !arm64

package transfer

import "sync"

var (
	detectOnce sync.Once
	detected   string
)

func detectOptimalCompression() string {
	detectOnce.Do(func() {
		detected = benchmarkCompression()
	})
	return detected
}
