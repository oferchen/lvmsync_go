package transfer

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"

	"lvmsync_go/config"
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
	d := NewCDCDedup(cfg)
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

func TestCDCDedupSaveStateWriteFailure(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	d := NewCDCDedup(cfg)
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
	d := NewCDCDedup(cfg)
	expected := 1 << (cfg.BloomMBits - 3)
	if len(d.index) != expected {
		t.Fatalf("unexpected index size %d want %d", len(d.index), expected)
	}
}
