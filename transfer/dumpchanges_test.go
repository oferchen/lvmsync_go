package transfer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"lvmsync_go/common"
	"lvmsync_go/device"
	"lvmsync_go/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// helper to create source and metadata files with deterministic ranges
func createDumpTestFiles(t testing.TB, blockSize int64, changedBlocks []int) (src, snapshot string) {
	t.Helper()
	snapshot = "testvg-testlv"
	_, src = createVolumeFiles(t, snapshot, blockSize, changedBlocks)
	return src, snapshot
}

func parseOffsetsNoHandshake(t *testing.T, r io.Reader) []int64 {
	t.Helper()
	reader := bufio.NewReader(r)
	var offsets []int64
	for {
		header := make([]byte, 16)
		if _, err := io.ReadFull(reader, header); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			t.Fatalf("failed to read header: %v", err)
		}
		off := int64(binary.BigEndian.Uint64(header[0:8]))
		size := int(binary.BigEndian.Uint32(header[8:12]))
		if _, err := io.CopyN(io.Discard, reader, int64(size)); err != nil {
			t.Fatalf("failed to read block data: %v", err)
		}
		offsets = append(offsets, off)
	}
	return offsets
}

func TestDumpChangesSequential(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, device.NewInfoWithDeps(nil, nil, func(context.Context, string) (bool, error) { return false, nil }, nil, nil))
	blockSize := int64(1024)
	changed := []int{0, 2}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", DedupStateFile: filepath.Join(t.TempDir(), "state"), DedupStrategy: "checksum", MaxRetries: 1}
	var buf bytes.Buffer
	if err := tr.DumpChangesSequential(context.Background(), cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesSequential failed: %v", err)
	}
	reader := bufio.NewReader(bytes.NewReader(buf.Bytes()))
	var hs common.Handshake
	hs, err := common.ReadHandshake(reader)
	if err != nil {
		t.Fatalf("failed to read handshake: %v", err)
	}
	if hs.Compress != "none" {
		t.Fatalf("unexpected handshake %+v", hs)
	}
	offsets := parseOffsetsNoHandshake(t, reader)
	expected := []int64{0, 2 * blockSize}
	if !reflect.DeepEqual(offsets, expected) {
		t.Fatalf("unexpected offsets %v, want %v", offsets, expected)
	}
}

type dummyDedup struct{}

func (d *dummyDedup) ShouldTransfer(int64, []byte) bool { return true }
func (d *dummyDedup) RecordTransfer(int64, []byte)      {}
func (d *dummyDedup) SaveState() error                  { return nil }

func TestDumpChangesWithDeduplication(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	blockSize := int64(1024)
	changed := []int{1}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", MaxRetries: 1}
	var buf bytes.Buffer
	dedup := &dummyDedup{}
	if err := tr.DumpChangesWithDeduplication(context.Background(), cfg, snapshot, src, &buf, dedup); err != nil {
		t.Fatalf("DumpChangesWithDeduplication failed: %v", err)
	}
	reader := bufio.NewReader(bytes.NewReader(buf.Bytes()))
	var hs common.Handshake
	var err error
	hs, err = common.ReadHandshake(reader)
	if err != nil {
		t.Fatalf("failed to read handshake: %v", err)
	}
	if !hs.ChecksumDedup || hs.Compress != "none" {
		t.Fatalf("unexpected handshake %+v", hs)
	}
	offsets := parseOffsetsNoHandshake(t, reader)
	expected := []int64{1 * blockSize}
	if !reflect.DeepEqual(offsets, expected) {
		t.Fatalf("unexpected offsets %v, want %v", offsets, expected)
	}
}

func TestDumpChangesParallelProgress(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	tr := NewTransfer(zap.New(core), &sync.WaitGroup{}, nil)
	blockSize := int64(1024)
	changed := []int{0}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", Parallel: 1, MaxRetries: 1, Progress: true}
	var buf bytes.Buffer
	if err := tr.DumpChangesParallel(context.Background(), cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesParallel failed: %v", err)
	}
	if logs.FilterMessage("transfer progress").Len() == 0 {
		t.Fatalf("expected progress log, got %d entries", logs.Len())
	}
}

func TestProcessDumpDataAutoDecompression(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, device.NewInfoWithDeps(nil, nil, func(context.Context, string) (bool, error) { return false, nil }, nil, nil))
	blockSize := int64(1024)
	changed := []int{0}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	cfgDump := &config.Config{BlockSize: int(blockSize), Compress: "zstd", ZstdLevel: 1, CompressLevel: 1, VerifyChecksum: true, Parallel: 1, MaxRetries: 1}
	var buf bytes.Buffer
	if err := tr.DumpChangesParallel(context.Background(), cfgDump, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesParallel failed: %v", err)
	}

	data := buf.Bytes()
	reader := bufio.NewReader(bytes.NewReader(data))
	hs, err := common.ReadHandshake(reader)
	if err != nil {
		t.Fatalf("failed to read handshake: %v", err)
	}
	if !hs.Checksum || hs.Compress != "zstd" {
		t.Fatalf("unexpected handshake %+v", hs)
	}

	dest := filepath.Join(t.TempDir(), "dest")
	info, err := os.Stat(src)
	if err != nil {
		t.Fatalf("stat source: %v", err)
	}
	destFile, err := os.Create(dest)
	if err != nil {
		t.Fatalf("create dest: %v", err)
	}
	if err = destFile.Truncate(info.Size()); err != nil {
		t.Fatalf("truncate dest: %v", err)
	}
	destFile.Close()

	cfgProcess := &config.Config{BlockSize: int(blockSize), Compress: "zstd,lz4", MaxRetries: 1}
	if err = tr.ProcessDumpData(context.Background(), cfgProcess, bytes.NewReader(data), dest); err != nil {
		t.Fatalf("ProcessDumpData failed: %v", err)
	}

	outFile, err := os.Open(dest)
	if err != nil {
		t.Fatalf("open dest: %v", err)
	}
	got := make([]byte, blockSize)
	if _, err = outFile.ReadAt(got, 0); err != nil {
		t.Fatalf("read dest: %v", err)
	}
	outFile.Close()
	want := bytes.Repeat([]byte{1}, int(blockSize))
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected data %v, want %v", got[:4], want[:4])
	}
}

func TestDumpChangesSequentialCanceled(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	blockSize := int64(1024)
	changed := []int{0, 1}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)
	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", MaxRetries: 1}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tr.DumpChangesSequential(ctx, cfg, snapshot, src, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestDumpChangesParallelCanceled(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	blockSize := int64(1024)
	changed := []int{0, 1}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)
	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", Parallel: 1, MaxRetries: 1}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tr.DumpChangesParallel(ctx, cfg, snapshot, src, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}
