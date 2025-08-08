package transfer

import (
	"bufio"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lvmsync_go/config"
)

func TestIterateBlocksOversizedOffset(t *testing.T) {
	src := newTempFile(t, "src")
	defer src.Close()

	cfg := &config.Config{BlockSize: 4096}
	bufOut := bufio.NewWriter(io.Discard)
	_, _, err := iterateBlocks(cfg, []Range{{Start: -1, End: 0}}, src, bufOut, nil, [2]int{-1, -1})
	if err == nil || !strings.Contains(err.Error(), "offset") {
		t.Fatalf("expected offset error, got %v", err)
	}
}

func TestIterateBlocksOversizedBlockSize(t *testing.T) {
	src := newTempFile(t, "src")
	defer src.Close()

	cfg := &config.Config{BlockSize: int(math.MaxUint32) + 1}
	bufOut := bufio.NewWriter(io.Discard)
	_, _, err := iterateBlocks(cfg, []Range{{Start: 0, End: 0}}, src, bufOut, nil, [2]int{-1, -1})
	if err == nil || !strings.Contains(err.Error(), "block size") {
		t.Fatalf("expected block size error, got %v", err)
	}
}

func TestSaveResumeStatePermissions(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{ResumeState: filepath.Join(dir, "resume")}
	saveResumeState(cfg, 0)

	info, err := os.Stat(cfg.ResumeState)
	if err != nil {
		t.Fatalf("stat resume state: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected permissions 600, got %v", info.Mode().Perm())
	}
}
