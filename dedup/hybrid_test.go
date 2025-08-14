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

func TestHybridChunkerBufferReuse(t *testing.T) {
	fixed := 1 << 16
	data := bytes.Repeat([]byte("a"), fixed*2)
	h := NewHybridChunker(fixed, 64, 128, 256)
	r := bytes.NewReader(data)

	c1, err := h.NextChunk(r)
	if err != nil && err != io.EOF {
		t.Fatalf("next chunk: %v", err)
	}
	if !bytes.Equal(c1.Data, data[:c1.Length]) {
		t.Fatalf("unexpected chunk data")
	}
	chunkPtr := &c1.Data[0]
	bufPtr := &h.buf[0]

	c2, err := h.NextChunk(r)
	if err != nil && err != io.EOF {
		t.Fatalf("next chunk: %v", err)
	}
	if &c2.Data[0] != chunkPtr {
		t.Fatalf("chunk buffer not reused")
	}

	offset := c1.Length + c2.Length
	for len(h.pending) > 0 {
		c, err := h.NextChunk(r)
		if err != nil && err != io.EOF {
			t.Fatalf("next chunk: %v", err)
		}
		offset += c.Length
	}

	c3, err := h.NextChunk(r)
	if err != nil && err != io.EOF {
		t.Fatalf("next chunk: %v", err)
	}
	if &h.buf[0] != bufPtr {
		t.Fatalf("fixed buffer not reused")
	}
	if !bytes.Equal(c3.Data, data[offset:offset+c3.Length]) {
		t.Fatalf("unexpected chunk data")
	}
}
