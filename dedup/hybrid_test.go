package dedup

import (
	"bytes"
	"io"
	"testing"
)

func TestHybridChunkerProducesChunks(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 1<<20)
	h := NewHybridChunker(1<<16, 64, 128, 256)
	r := bytes.NewReader(data)
	count := 0
	for {
		c, err := h.NextChunk(r)
		if err == io.EOF && c.Length == 0 {
			break
		}
		if c.Length < 64 || c.Length > 256 {
			t.Fatalf("chunk out of bounds %d", c.Length)
		}
		count++
		if err == io.EOF {
			break
		}
	}
	if count == 0 {
		t.Fatalf("expected chunks")
	}
}
