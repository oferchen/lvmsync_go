package transfer

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// newTempFile creates a temporary file and fails the test on error.
func newTempFile(t *testing.T, pattern string) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), pattern)
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	return f
}

// createVolumeFiles creates a source file and metadata describing changed blocks.
// It also sets up a temporary mapper directory with a symlink for the snapshot metadata.
// The returned dir contains the created files.
func createVolumeFiles(t testing.TB, snapshot string, blockSize int64, changedBlocks []int) (dir, src string) {
	t.Helper()
	dir = t.TempDir()

	src = filepath.Join(dir, "src")
	srcFile, err := os.Create(src)
	if err != nil {
		t.Fatalf("failed to create source file: %v", err)
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
		if _, err = srcFile.Write(data); err != nil {
			t.Fatalf("failed to write block: %v", err)
		}
	}
	srcFile.Close()

	meta := filepath.Join(dir, "meta")
	metaFile, err := os.Create(meta)
	if err != nil {
		t.Fatalf("failed to create metadata file: %v", err)
	}
	if _, err = metaFile.Write(make([]byte, blockSize)); err != nil {
		t.Fatalf("failed to write metadata header: %v", err)
	}
	for _, b := range changedBlocks {
		buf := make([]byte, 16)
		binary.LittleEndian.PutUint64(buf[0:8], uint64(b))
		binary.LittleEndian.PutUint64(buf[8:16], uint64(b+1))
		if _, err = metaFile.Write(buf); err != nil {
			t.Fatalf("failed to write metadata entry: %v", err)
		}
	}
	if _, err = metaFile.Write(make([]byte, 16)); err != nil {
		t.Fatalf("failed to write metadata terminator: %v", err)
	}
	metaFile.Close()

	mapper := t.TempDir()
	SetMapperDir(mapper)
	link := filepath.Join(mapper, snapshot+"-cow")
	if err = os.Symlink(meta, link); err != nil {
		t.Fatalf("failed to create metadata symlink: %v", err)
	}
	t.Cleanup(func() {
		os.Remove(link)
		SetMapperDir("/dev/mapper")
	})

	return dir, src
}
