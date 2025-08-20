//go:build integration

package transfer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	privilege "lvmsync_go/internal/privilege"
)

func TestResumeVerifyMismatch(t *testing.T) {
	dir := t.TempDir()
	blockSize := 4096
	first := bytes.Repeat([]byte{1}, blockSize)
	second := bytes.Repeat([]byte{2}, blockSize)
	data := append(first, second...)
	dest := filepath.Join(dir, "dest")
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	man := filepath.Join(dir, "dest.man")
	buildManifest(t, dest, man, "uuid", uint64(len(data)))
	resume := filepath.Join(dir, "resume.state")
	w, _, err := OpenWAL(resume+".wal", uint64(len(data)), "uuid", 0, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: uint64(len(data))}); err != nil {
		t.Fatalf("append wal: %v", err)
	}
	w.Close()
	// Corrupt second block
	corrupt := bytes.Repeat([]byte{3}, blockSize)
	f, err := os.OpenFile(dest, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open dest: %v", err)
	}
	if _, err := f.WriteAt(corrupt, int64(blockSize)); err != nil {
		t.Fatalf("corrupt dest: %v", err)
	}
	f.Close()
	firstDigest := blake3.Sum256(first)
	detect := func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *device.Runner) (device.Device, error) {
		return &mockDevice{path: dest, size: uint64(len(data)), blockSize: uint64(blockSize)}, nil
	}
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "uuid", nil },
		nil,
		func(context.Context, string) (bool, error) { return false, nil },
		func(context.Context, string, uint64) ([32]byte, error) { return firstDigest, nil },
		detect,
	)
	tr := NewTransfer(zap.NewNop(), nil, info)
	cfg := &config.Config{
		BlockSize:         blockSize,
		ManifestPath:      man,
		ResumeState:       resume,
		ResumeVerify:      true,
		Compress:          "none",
		ChecksumAlgorithm: "sha256",
		MaxRetries:        1,
	}
	err = tr.ProcessDumpData(context.Background(), cfg, bytes.NewReader(minimalStream(t)), dest)
	if err == nil {
		t.Fatalf("expected verification failure")
	}
}
