package dedup

import (
	"bytes"
	"io"
	mrand "math/rand"
	"testing"
)

const (
	benchSize = 1 << 20 // 1 MiB
	fixedSize = 1 << 16 // 64 KiB
	cdcMin    = 4 << 10
	cdcAvg    = 8 << 10
	cdcMax    = 16 << 10
)

var (
	patterned = patternedData()
	random    = randomData()
	benchSink byte
)

func patternedData() []byte {
	b := make([]byte, benchSize)
	pattern := []byte("abcdefghijklmnop")
	for i := 0; i < benchSize; i += len(pattern) {
		copy(b[i:], pattern)
	}
	return b
}

func randomData() []byte {
	b := make([]byte, benchSize)
	src := mrand.New(mrand.NewSource(1))
	_, _ = src.Read(b)
	return b
}

func benchFixed(b *testing.B, data []byte) {
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var sum byte
		for off := 0; off < len(data); off += fixedSize {
			sum += data[(off+i)%len(data)]
		}
		benchSink = sum
	}
}

func benchCDC(b *testing.B, data []byte) {
	ch, err := NewChunker(cdcMin, cdcAvg, cdcMax)
	if err != nil {
		b.Fatalf("new chunker: %v", err)
	}
	r := bytes.NewReader(data)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.Reset(data)
		for {
			_, err := ch.NextChunk(r)
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatalf("next chunk: %v", err)
			}
		}
	}
}

func benchHybrid(b *testing.B, data []byte) {
	h, err := NewHybridChunker(fixedSize, cdcMin, cdcAvg, cdcMax)
	if err != nil {
		b.Fatalf("new hybrid chunker: %v", err)
	}
	r := bytes.NewReader(data)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.Reset(data)
		for {
			_, err := h.NextChunk(r)
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatalf("next chunk: %v", err)
			}
		}
	}
}

func BenchmarkFixedPatterned(b *testing.B)  { benchFixed(b, patterned) }
func BenchmarkFixedRandom(b *testing.B)     { benchFixed(b, random) }
func BenchmarkCDCPatterned(b *testing.B)    { benchCDC(b, patterned) }
func BenchmarkCDCRandom(b *testing.B)       { benchCDC(b, random) }
func BenchmarkHybridPatterned(b *testing.B) { benchHybrid(b, patterned) }
func BenchmarkHybridRandom(b *testing.B)    { benchHybrid(b, random) }
