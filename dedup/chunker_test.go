package dedup

import (
	"bytes"
	"io"
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
