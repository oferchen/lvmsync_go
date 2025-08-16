package transfer

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

func TestIterateBlocksFinalSHA(t *testing.T) {
	logger := zap.NewNop()
	blockSize := int64(1024)
	snapshot := "vg-lv"
	_, src := createVolumeFiles(t, snapshot, blockSize, []int{0})
	cfg := &config.Config{BlockSize: int(blockSize), Compress: "zstd", ZstdLevel: 1, CompressLevel: 1, MaxRetries: 1, ChecksumAlgorithm: "sha256"}
	ranges, err := gatherChangedRanges(snapshot, blockSize, logger)
	if err != nil {
		t.Fatalf("gather ranges: %v", err)
	}
	srcFile, err := os.Open(src)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	defer srcFile.Close()
	w, err := NewCompressionWriter(io.Discard, cfg.Compress, cfg.ZstdLevel, 1)
	if err != nil {
		t.Fatalf("compression writer: %v", err)
	}
	bufOut := bufio.NewWriter(w)
	_, _, digest, err := iterateBlocks(cfg, ranges, srcFile, bufOut, nil, [2]int{-1, -1}, logger, nil)
	if err != nil {
		t.Fatalf("iterateBlocks: %v", err)
	}
	bufOut.Flush()
	w.Close()
	raw := bytes.Repeat([]byte{1}, int(blockSize))
	want := sha256.Sum256(raw)
	if !bytes.Equal(digest, want[:]) {
		t.Fatalf("sha mismatch")
	}
	// verify file digest matches
	f, err := os.Open(src)
	if err != nil {
		t.Fatalf("open src for verify: %v", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("hash file: %v", err)
	}
	if !bytes.Equal(h.Sum(nil), digest) {
		t.Fatalf("file digest mismatch")
	}
}
