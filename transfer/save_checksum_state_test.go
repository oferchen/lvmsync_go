package transfer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSaveChecksumState(t *testing.T) {
	t.Run("no logging", func(t *testing.T) {
		filename := filepath.Join(t.TempDir(), "state")
		state := &ChecksumState{Checksums: make(map[uint64][]byte), Strategy: "sha256"}
		if err := SaveChecksumState(filename, state, zap.NewNop()); err != nil {
			t.Fatalf("SaveChecksumState returned error: %v", err)
		}
	})

	t.Run("close warning", func(t *testing.T) {
		core, observed := observer.New(zap.WarnLevel)
		logger := zap.New(core)
		state := &ChecksumState{Checksums: make(map[uint64][]byte), Strategy: "sha256"}
		ff := &fakeFile{
			chmodErr: errors.New("chmod fail"),
			closeErr: errors.New("close fail"),
		}
		err := saveChecksumState("ignored", state, logger, func(string) (checksumFile, error) { return ff, nil })
		if err == nil {
			t.Fatalf("expected SaveChecksumState error")
		}
		logs := observed.FilterMessage("Failed to close checksum state file").All()
		if len(logs) != 1 {
			t.Skipf("expected 1 warning, got %d", len(logs))
		}
	})
}

type fakeFile struct {
	chmodErr error
	closeErr error
}

func (f *fakeFile) Write(p []byte) (int, error) { return len(p), nil }
func (f *fakeFile) Chmod(os.FileMode) error     { return f.chmodErr }
func (f *fakeFile) Close() error                { return f.closeErr }
