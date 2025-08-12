package transfer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/zeebo/blake3"

	"lvmsync_go/common"
	"lvmsync_go/config"

	"go.uber.org/zap"
)

func createTestFiles(t *testing.T, blockSize int64, blockCount int) (srcPath, snapshot, resumePath string) {
	t.Helper()
	changed := make([]int, blockCount)
	for i := 0; i < blockCount; i++ {
		changed[i] = i
	}
	snapshot = "vg-lv"
	dir, srcPath := createVolumeFiles(t, snapshot, blockSize, changed)

	resumePath = filepath.Join(dir, "resume")
	digest := blake3.Sum256(bytes.Repeat([]byte{2}, int(blockSize)))
	if err := os.WriteFile(resumePath, []byte(hex.EncodeToString(digest[:])), 0644); err != nil {
		t.Fatalf("failed to write resume state: %v", err)
	}
	return srcPath, snapshot, resumePath
}

func parseOffsets(t *testing.T, data []byte, blockSize int64) []int64 {
	t.Helper()
	reader := bufio.NewReader(bytes.NewReader(data))
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read handshake: %v", err)
	}
	if strings.TrimSpace(line) != common.ProtocolVersion+" compress:none" {
		t.Fatalf("unexpected handshake %q", strings.TrimSpace(line))
	}
	var offsets []int64
	for {
		header := make([]byte, 12)
		if _, err = io.ReadFull(reader, header); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			t.Fatalf("failed to read header: %v", err)
		}
		off := int64(binary.BigEndian.Uint64(header[0:8]))
		size := int(binary.BigEndian.Uint32(header[8:12]))
		if _, err = io.CopyN(io.Discard, reader, int64(size)); err != nil {
			t.Fatalf("failed to read block data: %v", err)
		}
		offsets = append(offsets, off)
	}
	return offsets
}

func TestResumeSequential(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{})
	blockSize := int64(1024)
	src, snapshot, resume := createTestFiles(t, blockSize, 4)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", Parallel: 1, ResumeState: resume, MaxRetries: 1}

	var buf bytes.Buffer
	if err := tr.DumpChangesParallel(cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesParallel failed: %v", err)
	}
	finalizeResumeState(cfg, zap.NewNop())

	offsets := parseOffsets(t, buf.Bytes(), blockSize)
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	expected := []int64{2 * blockSize, 3 * blockSize}
	if !reflect.DeepEqual(offsets, expected) {
		t.Fatalf("unexpected offsets %v, want %v", offsets, expected)
	}

	// resume state file contains the digest of the last transferred chunk
	// to allow recovery after interruptions.
}

func TestResumeParallel(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{})
	blockSize := int64(1024)
	src, snapshot, resume := createTestFiles(t, blockSize, 4)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", Parallel: 2, ResumeState: resume, MaxRetries: 1}

	var buf bytes.Buffer
	if err := tr.DumpChangesParallel(cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesParallel failed: %v", err)
	}
	finalizeResumeState(cfg, zap.NewNop())

	offsets := parseOffsets(t, buf.Bytes(), blockSize)
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	expected := []int64{2 * blockSize, 3 * blockSize}
	if !reflect.DeepEqual(offsets, expected) {
		t.Fatalf("unexpected offsets %v, want %v", offsets, expected)
	}

	// resume state file contains the digest of the last transferred chunk
	// to allow recovery after interruptions.
}
