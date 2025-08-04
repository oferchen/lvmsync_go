package transfer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"lvmsync_go/config"

	"go.uber.org/zap"
)

func createTestFiles(t *testing.T, blockSize int64, blockCount int) (srcPath, snapshot, resumePath string) {
	t.Helper()

	dir := t.TempDir()

	// create source file with distinct block data
	srcPath = filepath.Join(dir, "src")
	srcFile, err := os.Create(srcPath)
	if err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	for i := 0; i < blockCount; i++ {
		data := bytes.Repeat([]byte{byte(i + 1)}, int(blockSize))
		if _, err := srcFile.Write(data); err != nil {
			t.Fatalf("failed to write block: %v", err)
		}
	}
	srcFile.Close()

	// create metadata file describing changed blocks
	metaPath := filepath.Join(dir, "meta")
	metaFile, err := os.Create(metaPath)
	if err != nil {
		t.Fatalf("failed to create metadata file: %v", err)
	}
	if _, err := metaFile.Write(make([]byte, blockSize)); err != nil {
		t.Fatalf("failed to write metadata header: %v", err)
	}
	for i := 0; i < blockCount; i++ {
		buf := make([]byte, 16)
		binary.LittleEndian.PutUint64(buf[0:8], uint64(i))
		binary.LittleEndian.PutUint64(buf[8:16], uint64(i+1))
		if _, err := metaFile.Write(buf); err != nil {
			t.Fatalf("failed to write metadata entry: %v", err)
		}
	}
	if _, err := metaFile.Write(make([]byte, 16)); err != nil {
		t.Fatalf("failed to write metadata terminator: %v", err)
	}
	metaFile.Close()

	// create symlink for GetMetadataDevice in a temporary mapper directory
	mapper := t.TempDir()
	SetMapperDir(mapper)
	linkPath := filepath.Join(mapper, "vg-lv-cow")
	if err := os.Symlink(metaPath, linkPath); err != nil {
		t.Fatalf("failed to create metadata symlink: %v", err)
	}
	t.Cleanup(func() {
		os.Remove(linkPath)
		SetMapperDir("/dev/mapper")
	})

	snapshot = "vg-lv"

	// prepare resume state file skipping first two blocks
	resumePath = filepath.Join(dir, "resume")
	if err := os.WriteFile(resumePath, []byte("2"), 0644); err != nil {
		t.Fatalf("failed to write resume state: %v", err)
	}

	return srcPath, snapshot, resumePath
}

func parseOffsets(t *testing.T, data []byte, blockSize int64) []int64 {
	t.Helper()
	reader := bufio.NewReader(bytes.NewReader(data))
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("failed to read handshake: %v", err)
	}
	var offsets []int64
	for {
		header := make([]byte, 12)
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

func TestResumeSequential(t *testing.T) {
	SetLogger(zap.NewNop())
	blockSize := int64(1024)
	src, snapshot, resume := createTestFiles(t, blockSize, 4)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", Parallel: 1, ResumeState: resume, MaxRetries: 1}

	var buf bytes.Buffer
	if err := DumpChangesParallel(cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesParallel failed: %v", err)
	}

	offsets := parseOffsets(t, buf.Bytes(), blockSize)
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	expected := []int64{2 * blockSize, 3 * blockSize}
	if !reflect.DeepEqual(offsets, expected) {
		t.Fatalf("unexpected offsets %v, want %v", offsets, expected)
	}

	content, err := os.ReadFile(resume)
	if err != nil {
		t.Fatalf("failed to read resume file: %v", err)
	}
	val, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		t.Fatalf("invalid resume value: %v", err)
	}
	if val < 3 || val > 4 {
		t.Fatalf("resume state not updated, got %d", val)
	}
}

func TestResumeParallel(t *testing.T) {
	SetLogger(zap.NewNop())
	blockSize := int64(1024)
	src, snapshot, resume := createTestFiles(t, blockSize, 4)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", Parallel: 2, ResumeState: resume, MaxRetries: 1}

	var buf bytes.Buffer
	if err := DumpChangesParallel(cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesParallel failed: %v", err)
	}

	offsets := parseOffsets(t, buf.Bytes(), blockSize)
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	expected := []int64{2 * blockSize, 3 * blockSize}
	if !reflect.DeepEqual(offsets, expected) {
		t.Fatalf("unexpected offsets %v, want %v", offsets, expected)
	}

	content, err := os.ReadFile(resume)
	if err != nil {
		t.Fatalf("failed to read resume file: %v", err)
	}
	val, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		t.Fatalf("invalid resume value: %v", err)
	}
	if val < 3 || val > 4 {
		t.Fatalf("resume state not updated, got %d", val)
	}
}
