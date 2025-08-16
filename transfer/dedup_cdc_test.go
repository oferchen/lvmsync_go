package transfer

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"lvmsync_go/internal/config"
)

type cdcFailingWriter struct {
	failAfter int
	writes    int
}

func (fw *cdcFailingWriter) Write(p []byte) (int, error) {
	if fw.writes == fw.failAfter {
		return 0, io.ErrClosedPipe
	}
	fw.writes++
	return len(p), nil
}

func (fw *cdcFailingWriter) Close() error { return nil }

func TestCDCDedupChunkAndHash(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	d, err := NewCDCDedup(cfg)
	if err != nil {
		t.Fatalf("NewCDCDedup: %v", err)
	}
	data := bytes.Repeat([]byte("a"), cfg.CDCMin*2)
	chunks, final, err := d.ChunkAndHash(data)
	if err != nil {
		t.Fatalf("ChunkAndHash: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatalf("expected chunks")
	}
	empty := [32]byte{}
	if final == empty {
		t.Fatalf("expected final hash")
	}
}

func TestCDCDedupChunkBoundaries(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	// Use small CDC sizes for predictable chunking.
	cfg.CDCMin = 64
	cfg.CDCAvg = 64
	cfg.CDCMax = 128

	cases := []struct {
		name string
		size int
	}{
		{"ltMin", 32},
		{"eqMin", 64},
		{"gtMax", 256},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := NewCDCDedup(cfg)
			if err != nil {
				t.Fatalf("NewCDCDedup: %v", err)
			}
			data := bytes.Repeat([]byte("a"), tc.size)
			chunks, _, err := d.ChunkAndHash(data)
			if err != nil {
				t.Fatalf("ChunkAndHash: %v", err)
			}
			var total int
			for i, ch := range chunks {
				total += ch.Length
				if ch.Length > cfg.CDCMax {
					t.Fatalf("chunk %d exceeded max", i)
				}
				if i < len(chunks)-1 && ch.Length < cfg.CDCMin {
					t.Fatalf("chunk %d below min", i)
				}
			}
			if total != tc.size {
				t.Fatalf("total %d != size %d", total, tc.size)
			}
		})
	}
}

func TestCDCDedupSaveStateWriteFailure(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	d, err := NewCDCDedup(cfg)
	if err != nil {
		t.Fatalf("NewCDCDedup: %v", err)
	}
	fw := &cdcFailingWriter{failAfter: 0}
	orig := createStateFile
	createStateFile = func(string) (io.WriteCloser, error) { return fw, nil }
	defer func() { createStateFile = orig }()
	if err := d.SaveState(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCDCDedupMmapIndex(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.BloomMBits = 8
	cfg.DedupStateFile = filepath.Join(t.TempDir(), "state")
	d, err := NewCDCDedup(cfg)
	if err != nil {
		t.Fatalf("NewCDCDedup: %v", err)
	}
	expected := 1 << (cfg.BloomMBits - 3)
	if len(d.index) != expected {
		t.Fatalf("unexpected index size %d want %d", len(d.index), expected)
	}
}

func TestCDCDedupSaveState(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.DedupStateFile = filepath.Join(t.TempDir(), "state")
	d, err := NewCDCDedup(cfg)
	if err != nil {
		t.Fatalf("NewCDCDedup: %v", err)
	}
	if err := d.SaveState(); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	info, err := os.Stat(cfg.DedupStateFile)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("expected non-empty state file")
	}
}
