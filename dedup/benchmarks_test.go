package dedup

import (
	"bytes"
	"io"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/hash"
)

var entropySink float64

// BenchmarkReplicator measures end-to-end throughput of the replication
// pipeline on random data.
func BenchmarkReplicator(b *testing.B) {
	data := bytes.Repeat([]byte("abcde12345"), 1<<15) // ~1MB
	ch := NewChunker(64, 256, 1024)
	h, err := hash.NewBlake3Hasher(nil)
	if err != nil {
		b.Fatalf("new hasher: %v", err)
	}
	bloom := NewBloom(1<<20, 0.001)
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		var dst bytes.Buffer
		repl := NewReplicator(ch, h, bloom, &dst, zap.NewNop())
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

// BenchmarkUpdateEntropyRing measures the cost of updating the entropy window
// using the ring buffer implementation.
func BenchmarkUpdateEntropyRing(b *testing.B) {
	c := &Chunker{}
	counts := [256]int{}
	for i := range c.window {
		c.window[i] = byte(i)
		counts[c.window[i]]++
	}
	b.ResetTimer()
	var res float64
	for i := 0; i < b.N; i++ {
		res += c.updateEntropy(byte(i), &counts)
	}
	entropySink = res
}

// BenchmarkUpdateEntropyCopy replicates the previous copy-based window update
// to compare against the ring buffer implementation.
func BenchmarkUpdateEntropyCopy(b *testing.B) {
	var window [64]byte
	counts := [256]int{}
	for i := range window {
		window[i] = byte(i)
		counts[window[i]]++
	}
	b.ResetTimer()
	var res float64
	for i := 0; i < b.N; i++ {
		out := window[0]
		copy(window[:63], window[1:])
		window[63] = byte(i)
		counts[out]--
		counts[byte(i)]++
		res += entropy(counts[:])
	}
	entropySink = res
}
