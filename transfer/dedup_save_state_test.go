package transfer

import (
	"crypto/sha256"
	"errors"
	"io"
	"testing"

	"github.com/bits-and-blooms/bloom/v3"
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

func TestChecksumDedupSaveStateBinaryWriteFailure(t *testing.T) {
	hash := sha256.Sum256([]byte("data"))
	c := &ChecksumDedup{stateFile: "ignored", hashes: map[int64][]byte{1: hash[:]}, strategy: &SHA256Checksum{}}
	fw := &failingWriter{failAfter: 0}
	orig := createStateFile
	createStateFile = func(string) (io.WriteCloser, error) { return fw, nil }
	defer func() { createStateFile = orig }()
	if err := c.SaveState(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestChecksumDedupSaveStateFileWriteFailure(t *testing.T) {
	hash := sha256.Sum256([]byte("data"))
	c := &ChecksumDedup{stateFile: "ignored", hashes: map[int64][]byte{1: hash[:]}, strategy: &SHA256Checksum{}}
	fw := &failingWriter{failAfter: 1}
	orig := createStateFile
	createStateFile = func(string) (io.WriteCloser, error) { return fw, nil }
	defer func() { createStateFile = orig }()
	if err := c.SaveState(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRollingHashDedupSaveStateBinaryWriteFailure(t *testing.T) {
	r := &RollingHashDedup{stateFile: "ignored", hashes: map[int64]uint64{1: 1}}
	fw := &failingWriter{failAfter: 0}
	orig := createStateFile
	createStateFile = func(string) (io.WriteCloser, error) { return fw, nil }
	defer func() { createStateFile = orig }()
	if err := r.SaveState(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRollingHashDedupSaveStateFileWriteFailure(t *testing.T) {
	r := &RollingHashDedup{stateFile: "ignored", hashes: map[int64]uint64{1: 1}}
	fw := &failingWriter{failAfter: 1}
	orig := createStateFile
	createStateFile = func(string) (io.WriteCloser, error) { return fw, nil }
	defer func() { createStateFile = orig }()
	if err := r.SaveState(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestBloomFilterDedupSaveStateWriteFailure(t *testing.T) {
	b := &BloomFilterDedup{filter: bloom.NewWithEstimates(1000, 0.01), stateFile: "ignored", strategy: &SHA256Checksum{}}
	b.RecordTransfer(0, []byte("data"))
	fw := &failingWriter{failAfter: 0}
	orig := createStateFile
	createStateFile = func(string) (io.WriteCloser, error) { return fw, nil }
	defer func() { createStateFile = orig }()
	if err := b.SaveState(); err == nil {
		t.Fatalf("expected error")
	}
}
