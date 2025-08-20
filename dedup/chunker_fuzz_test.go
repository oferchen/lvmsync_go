package dedup

import (
	"bytes"
	"io"
	"math"
	"testing"
)

// FuzzChunkerNextChunk feeds random data and seeds to Chunker.NextChunk.
func FuzzChunkerNextChunk(f *testing.F) {
	f.Add(make([]byte, 64), uint64(0))
	f.Fuzz(func(t *testing.T, data []byte, seed uint64) {
		if len(data) == 0 {
			t.Skip()
		}
		ch, err := NewChunker(64, 128, 256, seed)
		if err != nil {
			t.Fatalf("new chunker: %v", err)
		}
		r := bytes.NewReader(data)
		var lengths []int
		var sum int
		for {
			c, err := ch.NextChunk(r)
			if err == io.EOF && c.Length == 0 {
				break
			}
			if err != nil && err != io.EOF {
				t.Fatalf("next chunk: %v", err)
			}
			if err != io.EOF && c.Length < ch.Min {
				t.Fatalf("chunk length %d below min %d", c.Length, ch.Min)
			}
			if c.Length > ch.Max {
				t.Fatalf("chunk length %d above max %d", c.Length, ch.Max)
			}
			lengths = append(lengths, c.Length)
			sum += c.Length
			if err == io.EOF {
				break
			}
		}
		if len(lengths) < 2 {
			return
		}
		mean := float64(sum) / float64(len(lengths))
		if math.Abs(mean-float64(ch.Avg)) > float64(ch.Avg)*0.5 {
			t.Fatalf("mean %f deviates from expected %d", mean, ch.Avg)
		}
	})
}
