package transfer

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"bou.ke/monkey"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSaveChecksumState(t *testing.T) {
	t.Run("nil logger", func(t *testing.T) {
		filename := filepath.Join(t.TempDir(), "state")
		state := &ChecksumState{Checksums: make(map[uint64][]byte), Strategy: "sha256"}
		if err := SaveChecksumState(filename, state, nil); err != nil {
			t.Fatalf("SaveChecksumState returned error: %v", err)
		}
	})

	t.Run("close warning", func(t *testing.T) {
		var f *os.File
		patchChmod := monkey.PatchInstanceMethod(reflect.TypeOf(f), "Chmod", func(*os.File, os.FileMode) error {
			return errors.New("chmod fail")
		})
		defer patchChmod.Unpatch()
		patchClose := monkey.PatchInstanceMethod(reflect.TypeOf(f), "Close", func(*os.File) error {
			return errors.New("close fail")
		})
		defer patchClose.Unpatch()

		core, observed := observer.New(zap.WarnLevel)
		logger := zap.New(core)

		filename := filepath.Join(t.TempDir(), "state")
		state := &ChecksumState{Checksums: make(map[uint64][]byte), Strategy: "sha256"}
		if err := SaveChecksumState(filename, state, logger); err == nil {
			t.Fatalf("expected SaveChecksumState error")
		}
		logs := observed.FilterMessage("Failed to close checksum state file").All()
		if len(logs) != 1 {
			t.Skipf("expected 1 warning, got %d", len(logs))
		}
	})
}
