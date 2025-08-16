package transfer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/device"
	manifestpkg "lvmsync_go/manifest"
)

func TestIterateBlocksUsesManifest(t *testing.T) {
	dir := t.TempDir()
	dev, err := os.CreateTemp(dir, "dev-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	data := make([]byte, 8192)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if _, err := dev.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	dev.Close()
	prev := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "id", nil })
	defer device.SetUUIDFunc(prev)
	manPath := filepath.Join(dir, "dev.man")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manifestpkg.Rebuild(ctx, dev.Name(), manPath, zap.NewNop(), 0, false, 0, 0, 0, 0); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	idx, err := manifestpkg.Open(manPath)
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	if idx.ChunkCount() != 2 {
		t.Fatalf("expected 2 chunks, got %d", idx.ChunkCount())
	}
	idx.Close()
	src, err := os.Open(dev.Name())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	cfg := &config.Config{BlockSize: 4096, ChecksumAlgorithm: "sha256", ManifestPath: manPath, MaxRetries: 1}
	ranges := []Range{{Start: 0, End: 4096}, {Start: 4096, End: 8192}}
	buf := bufio.NewWriter(bytes.NewBuffer(nil))
	total, skipped, _, err := iterateBlocks(cfg, ranges, src, buf, nil, [2]int{-1, -1}, zap.NewNop(), nil)
	if err != nil {
		t.Fatalf("iterateBlocks: %v", err)
	}
	if total != 0 {
		t.Fatalf("expected 0 bytes transferred, got %d", total)
	}
	if skipped != 2 {
		t.Fatalf("expected 2 skipped blocks, got %d", skipped)
	}
}

func TestProcessDumpDataRejectsManifestMismatch(t *testing.T) {
	dir := t.TempDir()
	src, err := os.CreateTemp(dir, "src-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	data := make([]byte, 8192)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if _, err := src.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	src.Close()
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "id", nil })
	defer device.SetUUIDFunc(prevUUID)
	manPath := filepath.Join(dir, "src.man")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manifestpkg.Rebuild(ctx, src.Name(), manPath, zap.NewNop(), 0, false, 0, 0, 0, 0); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	tr := NewTransfer(zap.NewNop(), nil)
	cfg := &config.Config{BlockSize: 4096, ManifestPath: manPath, MaxRetries: 1, Compress: "none", ChecksumAlgorithm: "sha256"}

	t.Run("id mismatch", func(t *testing.T) {
		dest := filepath.Join(dir, "dest-id")
		if err := os.WriteFile(dest, make([]byte, 8192), 0o600); err != nil {
			t.Fatalf("write dest: %v", err)
		}
		prev := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "wrong", nil })
		defer device.SetUUIDFunc(prev)
		err := tr.ProcessDumpData(context.Background(), cfg, bytes.NewReader(minimalStream(t)), dest)
		if err == nil || !strings.Contains(err.Error(), "does not match manifest") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("size mismatch", func(t *testing.T) {
		dest := filepath.Join(dir, "dest-size")
		if err := os.WriteFile(dest, make([]byte, 4096), 0o600); err != nil {
			t.Fatalf("write dest: %v", err)
		}
		prev := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "id", nil })
		defer device.SetUUIDFunc(prev)
		err := tr.ProcessDumpData(context.Background(), cfg, bytes.NewReader(minimalStream(t)), dest)
		if err == nil || !strings.Contains(err.Error(), "size") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
