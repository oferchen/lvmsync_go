package transfer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/config"
)

func TestIterateBlocksOffsetOverflow(t *testing.T) {
	src := newTempFile(t, "src")
	defer src.Close()

	cfg := &config.Config{BlockSize: 4096}
	bufOut := bufio.NewWriter(io.Discard)
	_, _, _, err := iterateBlocks(context.Background(), cfg, []Range{{Start: math.MaxUint64, End: math.MaxUint64}}, src, bufOut, nil, [2]int{-1, -1}, zap.NewNop(), nil)
	if err == nil || !strings.Contains(err.Error(), "offset") {
		t.Fatalf("expected offset error, got %v", err)
	}
}

func TestIterateBlocksOversizedBlockSize(t *testing.T) {
	src := newTempFile(t, "src")
	defer src.Close()

	cfg := &config.Config{BlockSize: int(math.MaxUint32) + 1}
	bufOut := bufio.NewWriter(io.Discard)
	_, _, _, err := iterateBlocks(context.Background(), cfg, []Range{{Start: 0, End: 0}}, src, bufOut, nil, [2]int{-1, -1}, zap.NewNop(), nil)
	if err == nil || !strings.Contains(err.Error(), "block size") {
		t.Fatalf("expected block size error, got %v", err)
	}
}

func TestSaveResumeStatePermissions(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{ResumeState: filepath.Join(dir, "resume"), CheckpointBytes: 1}
	rt := &resumeTracker{}
	saveResumeState(cfg, rt, 0, [32]byte{}, 1, zap.NewNop())

	info, err := os.Stat(cfg.ResumeState + ".wal")
	if err != nil {
		t.Fatalf("stat resume state: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected permissions 600, got %v", info.Mode().Perm())
	}
}

func TestReadBlockHeaderOffsetBoundary(t *testing.T) {
	header := make([]byte, 16)
	offset := uint64(math.MaxInt64)
	binary.BigEndian.PutUint64(header[0:8], offset)
	binary.BigEndian.PutUint32(header[8:12], 1)
	binary.BigEndian.PutUint32(header[12:16], 0)
	reader := bufio.NewReader(bytes.NewReader(header))
	gotOffset, gotSize, _, _, err := readBlockHeader(reader, make([]byte, 16), false, nil)
	if err != nil {
		t.Fatalf("readBlockHeader returned error: %v", err)
	}
	if gotOffset != offset || gotSize != 1 {
		t.Fatalf("unexpected header values %d %d", gotOffset, gotSize)
	}
}

func TestReadBlockHeaderOffsetOverflow(t *testing.T) {
	header := make([]byte, 16)
	offset := uint64(math.MaxUint64)
	binary.BigEndian.PutUint64(header[0:8], offset)
	binary.BigEndian.PutUint32(header[8:12], 1)
	binary.BigEndian.PutUint32(header[12:16], 0)
	reader := bufio.NewReader(bytes.NewReader(header))
	if _, _, _, _, err := readBlockHeader(reader, make([]byte, 16), false, nil); err == nil {
		t.Fatalf("expected overflow error")
	}
}

func TestWriteDataOffsetOverflow(t *testing.T) {
	dest := newTempFile(t, "dest")
	defer dest.Close()
	err := writeData(dest, math.MaxUint64, []byte("data"), zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "overflows int64") {
		t.Fatalf("expected overflow error, got %v", err)
	}
}
