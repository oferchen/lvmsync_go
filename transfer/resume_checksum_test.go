package transfer

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)

func TestResumeFinalChecksum(t *testing.T) {
	logger := zap.NewNop()
	blockSize := int64(1024)
	src, snapshot, resume := createTestFiles(t, blockSize, 4, "sha256")
	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", ResumeState: resume, MaxRetries: 1, ChecksumAlgorithm: "sha256", Transport: "ssh", DedupMode: "fixed"}
	ranges, err := gatherChangedRanges(snapshot, blockSize, logger)
	if err != nil {
		t.Fatalf("gather ranges: %v", err)
	}
	srcFile, err := os.Open(src)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	defer srcFile.Close()
	checkpoint := readResumeState(cfg, logger)
	start := findResumeIndex(cfg, srcFile, ranges, checkpoint, logger)
	w := bufio.NewWriter(io.Discard)
	_, _, digest, err := iterateBlocks(cfg, ranges[start:], srcFile, w, nil, [2]int{-1, -1}, logger, nil)
	if err != nil {
		t.Fatalf("iterateBlocks: %v", err)
	}
	w.Flush()
	raw1 := bytes.Repeat([]byte{3}, int(blockSize))
	raw2 := bytes.Repeat([]byte{4}, int(blockSize))
	h := sha256.New()
	h.Write(raw1)
	h.Write(raw2)
	if !bytes.Equal(digest, h.Sum(nil)) {
		t.Fatalf("verify after resume failed")
	}
}
