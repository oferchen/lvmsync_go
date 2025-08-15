package dedup

import (
	"bytes"
	"io"
	"math"
	"math/rand"
	"testing"
)

func TestChunkSizeDistribution(t *testing.T) {
	data := make([]byte, 4<<20)
	if _, err := rand.New(rand.NewSource(0)).Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}

	ch := NewChunker(64, 128, 256)
	r := bytes.NewReader(data)

	var lengths []int
	for {
		c, err := ch.NextChunk(r)
		if err == io.EOF && c.Length == 0 {
			break
		}
		if err != nil && err != io.EOF {
			t.Fatalf("next chunk: %v", err)
		}
		lengths = append(lengths, c.Length)
		if err == io.EOF {
			break
		}
	}

	if len(lengths) == 0 {
		t.Fatalf("expected chunks")
	}

	var sum, below, above int
	for _, l := range lengths {
		sum += l
		if l < ch.Avg {
			below++
		} else if l > ch.Avg {
			above++
		}
	}

	mean := float64(sum) / float64(len(lengths))
	if math.Abs(mean-float64(ch.Avg)) > float64(ch.Avg)*0.35 {
		t.Fatalf("mean %f out of expected range", mean)
	}
	if below == 0 || above == 0 {
		t.Fatalf("expected chunks both below and above average, got below=%d above=%d", below, above)
	}
}
