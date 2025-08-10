package dedup

import (
	"bytes"
	"io"
	"testing"
)

// BenchmarkReplicator measures end-to-end throughput of the replication
// pipeline on random data.
func BenchmarkReplicator(b *testing.B) {
	data := bytes.Repeat([]byte("abcde12345"), 1<<15) // ~1MB
	ch := NewChunker(64, 256, 1024)
	h, err := NewHasher(nil)
	if err != nil {
		b.Fatalf("new hasher: %v", err)
	}
	bloom := NewBloom(1<<20, 0.001)
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		var dst bytes.Buffer
		repl := NewReplicator(ch, h, bloom, &dst)
		if _, err := repl.Process(r); err != nil {
			b.Fatalf("process: %v", err)
		}
	}
}

// BenchmarkChunker measures the latency of chunk detection alone.
func BenchmarkChunker(b *testing.B) {
	data := bytes.Repeat([]byte("a"), 1<<20)
	for i := 0; i < b.N; i++ {
		ch := NewChunker(64, 256, 1024)
		r := bytes.NewReader(data)
		for {
			c, err := ch.NextChunk(r)
			if err == io.EOF && c.Length == 0 {
				break
			}
			if err == io.EOF {
				break
			}
		}
	}
}
