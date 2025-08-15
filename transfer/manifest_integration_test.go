package transfer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
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
	if err := manifestpkg.Rebuild(ctx, dev.Name(), manPath, zap.NewNop(), 0); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	src, err := os.Open(dev.Name())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	cfg := &config.Config{BlockSize: 4096, ChecksumAlgorithm: "sha256", ManifestPath: manPath, MaxRetries: 1}
	ranges := []Range{{Start: 0, End: 4096}, {Start: 4096, End: 8192}}
	buf := bufio.NewWriter(bytes.NewBuffer(nil))
	total, skipped, _, err := iterateBlocks(cfg, ranges, src, buf, nil, [2]int{-1, -1}, zap.NewNop())
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
