package dedup

import (
	"encoding/binary"
	"fmt"
	mathrand "math/rand"
	"testing"
)

// BenchmarkBloomFalsePositiveRate measures the observed false positive rate of
// the Bloom filter against its configured rate. It inserts a large number of
// unique digests and then tests randomly generated digests that were not
// inserted. The benchmark reports the observed false positive rate as an extra
// metric and fails if the rate exceeds twice the configured value.
func BenchmarkBloomFalsePositiveRate(b *testing.B) {
	const insertCount = 100000
	digests := make([][]byte, insertCount)
	for i := 0; i < insertCount; i++ {
		d := make([]byte, 32)
		binary.LittleEndian.PutUint64(d, uint64(i))
		digests[i] = d
	}

	rates := []float64{0.1, 0.01, 0.001}
	for _, rate := range rates {
		b.Run(fmt.Sprintf("fp%g", rate), func(b *testing.B) {
			bloom, err := NewBloom(uint(insertCount), rate)
			if err != nil {
				b.Fatalf("new bloom: %v", err)
			}
			for _, d := range digests {
				// Ignore return value; false positives are expected as the
				// filter fills up.
				bloom.TestAndAdd(d)
			}

			testRng := mathrand.New(mathrand.NewSource(2))
			b.ResetTimer()
			falsePos := 0
			for i := 0; i < b.N; i++ {
				d := make([]byte, 32)
				if _, err := testRng.Read(d); err != nil {
					b.Fatalf("rng read: %v", err)
				}
				if bloom.filter.Test(d) {
					falsePos++
				}
			}
			observed := float64(falsePos) / float64(b.N)
			b.ReportMetric(observed, "false_positive_rate")
			if observed > rate*2 {
				b.Fatalf("observed false positive rate %.4f exceeds threshold for configured %.4f", observed, rate)
			}
		})
	}
}
