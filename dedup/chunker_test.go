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
	ch := NewChunker(64, 128, 256)
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
