package transfer

import (
	"os"
	"testing"

	"go.uber.org/zap"
)

// TestWriteDataSeekError ensures writeData returns an error when seeking fails
// and that no data is written to the destination file.
func TestWriteDataSeekError(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "dest")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := f.Name()
	// Close the file to force Seek to fail
	f.Close()

	err = writeData(f, 0, []byte("data"), zap.NewNop())
	if err == nil {
		t.Fatalf("expected seek error")
	}

	reopened, err := os.Open(path)
	if err != nil {
		t.Fatalf("reopen file: %v", err)
	}
	defer reopened.Close()
	info, err := reopened.Stat()
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("expected no data written, got %d bytes", info.Size())
	}
}
