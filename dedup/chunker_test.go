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
