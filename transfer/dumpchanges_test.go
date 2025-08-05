package transfer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"lvmsync_go/common"
	"lvmsync_go/config"

	"go.uber.org/zap"
)

// helper to create source and metadata files with deterministic ranges
func createDumpTestFiles(t testing.TB, blockSize int64, changedBlocks []int) (src, snapshot string) {
	t.Helper()
	dir := t.TempDir()

	// create source file with distinct block data
	src = filepath.Join(dir, "src")
	srcFile, err := os.Create(src)
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	blockCount := 0
	for _, b := range changedBlocks {
		if b+1 > blockCount {
			blockCount = b + 1
		}
	}
	if blockCount < len(changedBlocks) {
		blockCount = len(changedBlocks)
	}
	for i := 0; i < blockCount; i++ {
		data := bytes.Repeat([]byte{byte(i + 1)}, int(blockSize))
		if _, err := srcFile.Write(data); err != nil {
			t.Fatalf("failed to write block: %v", err)
		}
	}
	srcFile.Close()

	// create metadata file describing changed blocks
	meta := filepath.Join(dir, "meta")
	metaFile, err := os.Create(meta)
	if err != nil {
		t.Fatalf("failed to create metadata: %v", err)
	}
	if _, err := metaFile.Write(make([]byte, blockSize)); err != nil {
		t.Fatalf("failed to write metadata header: %v", err)
	}
	for _, b := range changedBlocks {
		buf := make([]byte, 16)
		binary.LittleEndian.PutUint64(buf[0:8], uint64(b))
		binary.LittleEndian.PutUint64(buf[8:16], uint64(b+1))
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
	snapshot = "testvg-testlv"
	link := filepath.Join(mapper, "testvg-testlv-cow")
	if err := os.Symlink(meta, link); err != nil {
		t.Fatalf("failed to create metadata symlink: %v", err)
	}
	t.Cleanup(func() {
		os.Remove(link)
		SetMapperDir("/dev/mapper")
	})

	return src, snapshot
}

func parseOffsetsNoHandshake(t *testing.T, r io.Reader) []int64 {
	t.Helper()
	reader := bufio.NewReader(r)
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

func TestDumpChangesSequential(t *testing.T) {
	SetLogger(zap.NewNop())
	blockSize := int64(1024)
	changed := []int{0, 2}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", DedupStateFile: filepath.Join(t.TempDir(), "state"), DedupStrategy: "checksum", MaxRetries: 1}
	var buf bytes.Buffer
	if err := DumpChangesSequential(cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesSequential failed: %v", err)
	}
	reader := bufio.NewReader(bytes.NewReader(buf.Bytes()))
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read handshake: %v", err)
	}
	if strings.TrimSpace(line) != common.ProtocolVersion+" compress:none" {
		t.Fatalf("unexpected handshake %q", strings.TrimSpace(line))
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
	SetLogger(zap.NewNop())
	blockSize := int64(1024)
	changed := []int{1}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", MaxRetries: 1}
	var buf bytes.Buffer
	dedup := &dummyDedup{}
	if err := DumpChangesWithDeduplication(cfg, snapshot, src, &buf, dedup); err != nil {
		t.Fatalf("DumpChangesWithDeduplication failed: %v", err)
	}
	reader := bufio.NewReader(bytes.NewReader(buf.Bytes()))
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read handshake: %v", err)
	}
	if strings.TrimSpace(line) != common.ProtocolVersion+" checksum-dedup compress:none" {
		t.Fatalf("unexpected handshake %q", strings.TrimSpace(line))
	}
	offsets := parseOffsetsNoHandshake(t, reader)
	expected := []int64{1 * blockSize}
	if !reflect.DeepEqual(offsets, expected) {
		t.Fatalf("unexpected offsets %v, want %v", offsets, expected)
	}
}

func TestProcessDumpDataAutoDecompression(t *testing.T) {
	SetLogger(zap.NewNop())
	blockSize := int64(1024)
	changed := []int{0}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	cfgDump := &config.Config{BlockSize: int(blockSize), Compress: "zstd", CompressLevel: 1, VerifyChecksum: true, Parallel: 1, MaxRetries: 1}
	var buf bytes.Buffer
	if err := DumpChangesParallel(cfgDump, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesParallel failed: %v", err)
	}

	data := buf.Bytes()
	reader := bufio.NewReader(bytes.NewReader(data))
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read handshake: %v", err)
	}
	if strings.TrimSpace(line) != common.ProtocolVersion+" checksum compress:zstd" {
		t.Fatalf("unexpected handshake %q", strings.TrimSpace(line))
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
	if err := destFile.Truncate(info.Size()); err != nil {
		t.Fatalf("truncate dest: %v", err)
	}
	destFile.Close()

	cfgProcess := &config.Config{BlockSize: int(blockSize), Compress: "none", MaxRetries: 1}
	if err := ProcessDumpData(cfgProcess, bytes.NewReader(data), dest); err != nil {
		t.Fatalf("ProcessDumpData failed: %v", err)
	}

	outFile, err := os.Open(dest)
	if err != nil {
		t.Fatalf("open dest: %v", err)
	}
	got := make([]byte, blockSize)
	if _, err := outFile.ReadAt(got, 0); err != nil {
		t.Fatalf("read dest: %v", err)
	}
	outFile.Close()
	want := bytes.Repeat([]byte{1}, int(blockSize))
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected data %v, want %v", got[:4], want[:4])
	}
}
