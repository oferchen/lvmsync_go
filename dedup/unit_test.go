package dedup_test

import (
	"bytes"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/oferchen/lvmsync_go/dedup"
	"github.com/oferchen/lvmsync_go/hash"
	"github.com/oferchen/lvmsync_go/internal/config"
	transfer "github.com/oferchen/lvmsync_go/transfer"
)

func TestHasher(t *testing.T) {
	h, err := hash.NewBlake3Hasher(nil)
	if err != nil {
		t.Fatalf("new hasher: %v", err)
	}
	msg := []byte("hello world")
	if _, err = h.Write(msg); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	sum := h.Sum256()
	expected := "d74981efa70a0c880b8d8c1985d075dbcbf679b99a5f9914e5aaf96b831a9e24"
	if hex.EncodeToString(sum[:]) != expected {
		t.Fatalf("unexpected hash %x", sum)
	}

	key := []byte("0123456789abcdef0123456789abcdef")
	h, err = hash.NewBlake3Hasher(key)
	if err != nil {
		t.Fatalf("new hasher: %v", err)
	}
	if _, err = h.Write(msg); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	sum2 := h.Sum256()
	if bytes.Equal(sum[:], sum2[:]) {
		t.Fatalf("keyed hash should differ")
	}
}

func TestBloom(t *testing.T) {
	b, err := dedup.NewBloom(1000, 0.01)
	if err != nil {
		t.Fatalf("new bloom: %v", err)
	}
	data := []byte("chunk")
	if b.TestAndAdd(data) {
		t.Fatalf("unexpected hit")
	}
	if !b.TestAndAdd(data) {
		t.Fatalf("expected hit on second insert")
	}
}

func TestBloomInvalidFpRate(t *testing.T) {
	cases := []float64{0, 1, -0.5, 1.5}
	for _, fp := range cases {
		if _, err := dedup.NewBloom(1000, fp); err == nil {
			t.Fatalf("expected error for fpRate %v", fp)
		}
	}
}

func TestChunkerBounds(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 1<<20)
	ch, err := dedup.NewChunker(64, 128, 256)
	if err != nil {
		t.Fatalf("new chunker: %v", err)
	}
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

func TestFastCDC(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 1<<16)
	chunks, err := dedup.FastCDC(bytes.NewReader(data), 64, 128, 256)
	if err != nil {
		t.Fatalf("FastCDC failed: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatalf("expected chunks")
	}
	for _, c := range chunks {
		if c.Length < 64 || c.Length > 256 {
			t.Fatalf("chunk out of bounds %d", c.Length)
		}
	}
}

func TestBloomSizing(t *testing.T) {
	max, err := dedup.MaxChunks(1<<30, 0.01)
	if err != nil {
		t.Fatalf("MaxChunks: %v", err)
	}
	if max == 0 {
		t.Fatalf("expected non-zero max chunks")
	}
	avg, chunks, err := dedup.AdaptiveAvgChunk(1<<40, 1<<30, 0.01, 64, 1<<20)
	if err != nil {
		t.Fatalf("AdaptiveAvgChunk: %v", err)
	}
	if avg < 64 || chunks == 0 {
		t.Fatalf("unexpected sizing avg=%d chunks=%d", avg, chunks)
	}
	if _, err := dedup.MaxChunks(1<<30, 0); err == nil {
		t.Fatalf("expected error for invalid fpRate in MaxChunks")
	}
	if _, _, err := dedup.AdaptiveAvgChunk(1<<40, 1<<30, 0, 64, 1<<20); err == nil {
		t.Fatalf("expected error for invalid fpRate in AdaptiveAvgChunk")
	}
}

func TestChunkerBufferReuse(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 1<<10)
	ch, err := dedup.NewChunker(64, 128, 256)
	if err != nil {
		t.Fatalf("new chunker: %v", err)
	}
	r := bytes.NewReader(data)

	c1, err := ch.NextChunk(r)
	if err != nil && err != io.EOF {
		t.Fatalf("next chunk: %v", err)
	}
	if !bytes.Equal(c1.Data, data[:c1.Length]) {
		t.Fatalf("unexpected chunk data")
	}
	ptr := &c1.Data[0]

	c2, err := ch.NextChunk(r)
	if err != nil && err != io.EOF {
		t.Fatalf("next chunk: %v", err)
	}
	if c2.Length == 0 {
		t.Fatalf("expected second chunk")
	}
	if &c2.Data[0] != ptr {
		t.Fatalf("buffer not reused")
	}
	if !bytes.Equal(c2.Data, data[c1.Length:c1.Length+c2.Length]) {
		t.Fatalf("unexpected chunk data")
	}
}

func TestBloomStateReuseDiscardsIndex(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{
		BloomEntries:   1000,
		BloomFpRate:    0.01,
		BloomMBits:     10,
		DedupStateFile: filepath.Join(tmp, "state"),
		CDCMin:         64,
		CDCAvg:         128,
		CDCMax:         256,
	}

	data := bytes.Repeat([]byte("a"), 1<<10)

	// First run populates the Bloom filter and index.
	cd1, err := transfer.NewCDCDedup(cfg)
	if err != nil {
		t.Fatalf("new cdcdedup: %v", err)
	}
	if _, _, err := cd1.ChunkAndHash(data); err != nil {
		t.Fatalf("chunk and hash: %v", err)
	}
	if err := cd1.SaveState(); err != nil {
		t.Fatalf("save state: %v", err)
	}
	idxPath := cfg.DedupStateFile + ".idx"
	content, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if bytes.Count(content, []byte{0}) == len(content) {
		t.Fatalf("expected index to contain data")
	}

	// Second run reuses Bloom state but should discard the previous index.
	if _, err := transfer.NewCDCDedup(cfg); err != nil {
		t.Fatalf("new cdcdedup reuse: %v", err)
	}
	content2, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatalf("read index after reuse: %v", err)
	}
	if bytes.Count(content2, []byte{0}) != len(content2) {
		t.Fatalf("stale index not discarded")
	}
}
