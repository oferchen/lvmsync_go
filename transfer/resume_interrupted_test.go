package transfer

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/device"
	"github.com/oferchen/lvmsync_go/internal/config"
)

func runInterrupted(t *testing.T, mode string) {
	t.Helper()
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	tr.Tracker = &resumeTracker{}
	blockSize := int64(1024)
	snapshot := "vg-lv"
	dir, src := createVolumeFiles(t, snapshot, blockSize, []int{0, 1, 2, 3})
	resume := filepath.Join(dir, "resume")

	cfg := &config.Config{
		BlockSize:         int(blockSize),
		Compress:          "none",
		Parallel:          1,
		ResumeState:       resume,
		MaxRetries:        1,
		ChecksumAlgorithm: "blake3",
		Transport:         "ssh",
		DedupMode:         mode,
		CheckpointBytes:   1,
	}

	// simulate interrupted transfer: process one successful result then an error
	results := make(chan *BlockResult, 2)
	data := bytes.Repeat([]byte{1}, int(blockSize))
	digest := blake3.Sum256(data)
	results <- &BlockResult{Index: 0, Offset: 0, Size: uint32(blockSize), Data: data, ChunkID: digest}
	results <- &BlockResult{Index: 1, Err: io.ErrUnexpectedEOF}
	close(results)
	checksum := GetChecksumStrategy(cfg.ChecksumAlgorithm)
	bufOut := bufio.NewWriter(io.Discard)
	if _, _, err := processParallelResults(context.Background(), cfg, results, bufOut, checksum, 0, time.Now(), zap.NewNop(), tr.Tracker); err == nil {
		t.Fatalf("expected error")
	}

	chk := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{FSUUID: cfg.DeviceUUID}, [32]byte{}).chunk(mode)
	if chk.Offset != 0 || chk.Length == 0 || chk.Chunk != digest {
		t.Fatalf("unexpected checkpoint %+v", chk)
	}

	var buf bytes.Buffer
	if err := tr.DumpChangesParallel(context.Background(), cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesParallel failed: %v", err)
	}
	finalizeResumeState(cfg, tr.Tracker, zap.NewNop())

	offsets := parseOffsets(t, buf.Bytes(), blockSize)
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	expectedOffsets := []int64{blockSize, 2 * blockSize, 3 * blockSize}
	if !reflect.DeepEqual(offsets, expectedOffsets) {
		t.Fatalf("unexpected offsets %v, want %v", offsets, expectedOffsets)
	}
}

func TestResumeInterruptedFixed(t *testing.T) {
	runInterrupted(t, "fixed")
}

func TestResumeInterruptedCDC(t *testing.T) {
	runInterrupted(t, "cdc")
}

func TestResumeInterruptedHybrid(t *testing.T) {
	runInterrupted(t, "hybrid")
}
