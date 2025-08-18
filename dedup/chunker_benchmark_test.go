package dedup

import (
	"bytes"
	"testing"
)

func BenchmarkChunkerNextChunk(b *testing.B) {
	data := make([]byte, 1024)
	ch, err := NewChunker(64, 128, 256)
	if err != nil {
		b.Fatalf("new chunker: %v", err)
	}
	ch.buf = make([]byte, ch.Max)
	r := bytes.NewReader(data)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.Reset(data)
		_, _ = ch.NextChunk(r)
	}
}
