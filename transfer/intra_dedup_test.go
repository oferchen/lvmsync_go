package transfer

import (
	"os"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
)

func TestChunkCache(t *testing.T) {
	c := newChunkCache(2)
	a := []byte("aaaa")
	b := []byte("bbbb")
	cSeen := c.Seen(a)
	if cSeen {
		t.Fatalf("first insert should be new")
	}
	if !c.Seen(a) {
		t.Fatalf("expected duplicate for a")
	}
	if c.Seen(b) {
		t.Fatalf("first insert of b should be new")
	}
	// Insert third unique chunk to evict a.
	if c.Seen([]byte("cccc")) {
		t.Fatalf("first insert of c should be new")
	}
	if c.Seen(b) == false {
		t.Fatalf("b should still be cached")
	}
	// a was evicted, should be treated as new
	if c.Seen(a) {
		t.Fatalf("a should have been evicted")
	}
}

func TestProcessBlockIntraDedup(t *testing.T) {
	cfg := &config.Config{}
	cache := newChunkCache(2)
	f := mustTempFile(t)
	defer os.Remove(f.Name())
	data := []byte("data")
	crc := crc32c(data)
	d := device.NewDiscarderWithFunc(func(*os.File, uint64, uint64, bool, bool, *zap.Logger) error { return nil })
	written, _, err := processBlock(cfg, f, nil, cache, false, nil, 0, crc, nil, data, uint32(len(data)), zap.NewNop(), nil, d)
	if err != nil || !written {
		t.Fatalf("first write %v %v", written, err)
	}
	written, _, err = processBlock(cfg, f, nil, cache, false, nil, uint64(len(data)), crc, nil, data, uint32(len(data)), zap.NewNop(), nil, d)
	if err != nil {
		t.Fatalf("second write err %v", err)
	}
	if written {
		t.Fatalf("duplicate block should not be written")
	}
}

func mustTempFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp("", "chunk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	return f
}
