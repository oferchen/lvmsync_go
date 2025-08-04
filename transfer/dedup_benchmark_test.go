package transfer

import (
	"sync/atomic"
	"testing"

	"github.com/bits-and-blooms/bloom/v3"
)

func BenchmarkBloomFilterShouldTransfer(b *testing.B) {
	d := &BloomFilterDedup{filter: bloom.NewWithEstimates(1000000, 0.01)}
	data := []byte("benchmark data")
	d.RecordTransfer(0, data)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			d.ShouldTransfer(0, data)
		}
	})
}

func BenchmarkBloomFilterRecordTransfer(b *testing.B) {
	d := &BloomFilterDedup{filter: bloom.NewWithEstimates(1000000, 0.01)}
	data := []byte("benchmark data")
	var offset int64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			o := atomic.AddInt64(&offset, 1)
			d.RecordTransfer(o, data)
		}
	})
}
