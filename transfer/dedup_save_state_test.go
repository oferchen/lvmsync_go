package transfer

import (
	"crypto/sha256"
	"errors"
	"io"
	"testing"
)

type failingWriter struct {
	failAfter int
	writes    int
}

func (fw *failingWriter) Write(p []byte) (int, error) {
	if fw.writes == fw.failAfter {
		return 0, errors.New("write failed")
	}
	fw.writes++
	return len(p), nil
}

func (fw *failingWriter) Close() error { return nil }

func TestChecksumDedupSaveStateWriteFailures(t *testing.T) {
	hash := sha256.Sum256([]byte("data"))

	t.Run("binary write failure", func(t *testing.T) {
		c := &ChecksumDedup{stateFile: "ignored", hashes: map[int64][32]byte{1: hash}}
		fw := &failingWriter{failAfter: 0}
		orig := createStateFile
		createStateFile = func(string) (io.WriteCloser, error) { return fw, nil }
		defer func() { createStateFile = orig }()
		if err := c.SaveState(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("file write failure", func(t *testing.T) {
		c := &ChecksumDedup{stateFile: "ignored", hashes: map[int64][32]byte{1: hash}}
		fw := &failingWriter{failAfter: 1}
		orig := createStateFile
		createStateFile = func(string) (io.WriteCloser, error) { return fw, nil }
		defer func() { createStateFile = orig }()
		if err := c.SaveState(); err == nil {
			t.Fatalf("expected error")
		}
	})
}
