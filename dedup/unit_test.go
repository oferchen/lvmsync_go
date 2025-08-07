package dedup

import (
	"bytes"
	"encoding/hex"
	"io"
	"testing"
)

func TestHasher(t *testing.T) {
	h := NewHasher(nil)
	msg := []byte("hello world")
	if _, err := h.Write(msg); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	sum := h.Sum256()
	expected := "d74981efa70a0c880b8d8c1985d075dbcbf679b99a5f9914e5aaf96b831a9e24"
	if hex.EncodeToString(sum[:]) != expected {
		t.Fatalf("unexpected hash %x", sum)
	}

	key := []byte("0123456789abcdef0123456789abcdef")
	h = NewHasher(key)
	if _, err := h.Write(msg); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	sum2 := h.Sum256()
	if bytes.Equal(sum[:], sum2[:]) {
		t.Fatalf("keyed hash should differ")
	}
}

func TestBloom(t *testing.T) {
	b := NewBloom(1000, 0.01)
	data := []byte("chunk")
	if b.TestAndAdd(data) {
		t.Fatalf("unexpected hit")
	}
	if !b.TestAndAdd(data) {
		t.Fatalf("expected hit on second insert")
	}
}

func TestChunkerBounds(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 1<<20)
	ch := NewChunker(64, 128, 256)
	r := bytes.NewReader(data)
	for {
		c, err := ch.NextChunk(r)
		if err == io.EOF && c.Length == 0 {
			break
		}
		if c.Length < 64 || c.Length > 256 {
			t.Fatalf("chunk out of bounds %d", c.Length)
		}
		if err == io.EOF {
			break
		}
	}
}
