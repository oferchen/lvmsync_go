package transfer

import "time"

// Estimate returns the expected duration in milliseconds and bandwidth in bits per second
// for transferring size bytes at the given speedLimit bytes per second.
// When speedLimit <= 0 or size <= 0, zero values are returned.
func Estimate(size int64, speedLimit int) (durationMs, bandwidthBps int64) {
	if size <= 0 || speedLimit <= 0 {
		return 0, 0
	}
	durationMs = int64(time.Duration(size/int64(speedLimit)) * time.Second / time.Millisecond)
	bandwidthBps = int64(speedLimit) * 8
	return
}
