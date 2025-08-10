package transport

import (
	"math"
	"sync"
	"time"
)

const defaultStreamSize = 64 * 1024 // 64 KiB per stream window

// BDPTracker estimates the bandwidth-delay product and recommends
// a stream concurrency that keeps roughly 1-2x BDP of data in flight.
// An optional override forces a fixed concurrency value.
type BDPTracker struct {
	mu          sync.Mutex
	streamSize  int64
	bdp         int64
	concurrency int
	override    int
}

// NewBDPTracker creates a tracker. streamSize defines the amount of data
// each stream keeps in flight. When override > 0 the tracker always returns
// that concurrency instead of auto-tuning.
func NewBDPTracker(streamSize int64, override int) *BDPTracker {
	if streamSize <= 0 {
		streamSize = defaultStreamSize
	}
	t := &BDPTracker{streamSize: streamSize, override: override}
	if override > 0 {
		t.concurrency = override
	} else {
		t.concurrency = 1
	}
	return t
}

// Update records an RTT sample and bandwidth (bytes per second) measurement
// and recalculates the recommended concurrency. The new concurrency is
// returned.
func (b *BDPTracker) Update(rtt time.Duration, bandwidth float64) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bdp = int64(bandwidth * rtt.Seconds())
	if b.override > 0 {
		b.concurrency = b.override
		return b.concurrency
	}
	b.concurrency = b.autotune()
	return b.concurrency
}

func (b *BDPTracker) autotune() int {
	if b.bdp <= 0 {
		return 1
	}
	ss := float64(b.streamSize)
	bdp := float64(b.bdp)
	lower := int(math.Ceil(bdp / ss))
	upper := int(math.Max(1, math.Floor(2*bdp/ss)))
	target := int(math.Ceil(1.5 * bdp / ss))
	if target < lower {
		target = lower
	}
	if target > upper {
		target = upper
	}
	if target < 1 {
		target = 1
	}
	return target
}

// Concurrency returns the current recommended concurrency.
func (b *BDPTracker) Concurrency() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.concurrency
}

// Override sets a fixed concurrency. A value <=0 re-enables auto-tuning.
func (b *BDPTracker) Override(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.override = n
	if n > 0 {
		b.concurrency = n
	} else {
		b.concurrency = b.autotune()
	}
}
