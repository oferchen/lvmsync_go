package dedup

import (
	"bytes"
	"io"
	"math/rand"
	"reflect"
	"testing"
)

func TestDeterministicBoundaries(t *testing.T) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	ch, err := NewChunker(64, 128, 256)
	if err != nil {
		t.Fatalf("new chunker: %v", err)
	}
	r := bytes.NewReader(data)

	var offsets []int
	var off int
	for {
		c, err := ch.NextChunk(r)
		if err == io.EOF && c.Length == 0 {
			break
		}
		if err != nil && err != io.EOF {
			t.Fatalf("next chunk: %v", err)
		}
		offsets = append(offsets, off)
		off += c.Length
		if err == io.EOF {
			break
		}
	}

	expected := []int{0, 119, 216, 293, 375, 472, 549, 631, 728, 805, 887, 984}
	if !reflect.DeepEqual(offsets, expected) {
		t.Fatalf("offsets %v != expected %v", offsets, expected)
	}
}

func TestNewChunkerValidation(t *testing.T) {
	tests := []struct {
		name          string
		min, avg, max int
		ok            bool
	}{
		{"valid", 64, 128, 256, true},
		{"zero", 0, 128, 256, false},
		{"negative", -1, 128, 256, false},
		{"too_small", 32, 128, 256, false},
		{"min>avg", 128, 64, 256, false},
		{"avg>max", 64, 256, 128, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch, err := NewChunker(tt.min, tt.avg, tt.max)
			if tt.ok {
				if err != nil {
					t.Fatalf("expected success got %v", err)
				}
				if ch == nil {
					t.Fatalf("expected chunker, got nil")
				}
			} else if err == nil {
				t.Fatalf("expected error for %v", tt)
			}
		})
	}
}

func TestFastCDCDataIsolation(t *testing.T) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	chunks, err := FastCDC(bytes.NewReader(data), 64, 128, 256)
	if err != nil {
		t.Fatalf("FastCDC failed: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least two chunks, got %d", len(chunks))
	}
	if len(chunks[1].Data) == 0 {
		t.Fatalf("second chunk has no data")
	}
	orig := chunks[1].Data[0]
	chunks[0].Data[0] ^= 0xFF
	if chunks[1].Data[0] != orig {
		t.Fatalf("modifying first chunk altered second chunk data")
	}
}

func TestChunkerSeedAffectsBoundaries(t *testing.T) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	chunks1, err := FastCDC(bytes.NewReader(data), 64, 128, 256, 1)
	if err != nil {
		t.Fatalf("FastCDC seed1: %v", err)
	}
	chunks2, err := FastCDC(bytes.NewReader(data), 64, 128, 256, 2)
	if err != nil {
		t.Fatalf("FastCDC seed2: %v", err)
	}
	if reflect.DeepEqual(chunks1, chunks2) {
		t.Fatalf("expected different chunk boundaries with different seeds")
	}
}

func TestFastCDCDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	const seed = 12345
	for i := 0; i < 5; i++ {
		size := rng.Intn(8192)
		data := make([]byte, size)
		if _, err := rng.Read(data); err != nil {
			t.Fatalf("random data: %v", err)
		}
		chunks1, err := FastCDC(bytes.NewReader(data), 64, 128, 256, seed)
		if err != nil {
			t.Fatalf("fastcdc run1: %v", err)
		}
		chunks2, err := FastCDC(bytes.NewReader(data), 64, 128, 256, seed)
		if err != nil {
			t.Fatalf("fastcdc run2: %v", err)
		}
		if len(chunks1) != len(chunks2) {
			t.Fatalf("chunk count mismatch: %d vs %d", len(chunks1), len(chunks2))
		}
		for j := range chunks1 {
			if chunks1[j].Offset != chunks2[j].Offset || chunks1[j].Length != chunks2[j].Length {
				t.Fatalf("chunk %d mismatch: %v vs %v", j, chunks1[j], chunks2[j])
			}
		}
	}
}

func TestChunkerNextChunkAllocations(t *testing.T) {
	data := make([]byte, 1024)
	ch, err := NewChunker(64, 128, 256)
	if err != nil {
		t.Fatalf("new chunker: %v", err)
	}
	ch.buf = make([]byte, ch.Max)
	r := bytes.NewReader(nil)
	allocs := testing.AllocsPerRun(100, func() {
		r.Reset(data)
		_, _ = ch.NextChunk(r)
	})
	if allocs > 1 {
		t.Fatalf("expected at most one allocation, got %f", allocs)
	}
}
