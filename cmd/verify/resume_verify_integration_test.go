//go:build integration

package verify

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"golang.org/x/sys/unix"
	"lvmsync_go/common"
	"lvmsync_go/device"
	hashutil "lvmsync_go/hash"
	"lvmsync_go/internal/config"
	privilege "lvmsync_go/internal/privilege"
	manifestpkg "lvmsync_go/manifest"
	"lvmsync_go/transfer"
)

type mockDevice struct {
	path      string
	size      uint64
	blockSize uint64
}

func (m *mockDevice) Path() string                                            { return m.path }
func (m *mockDevice) SizeBytes() uint64                                       { return m.size }
func (m *mockDevice) BlockSize() uint64                                       { return m.blockSize }
func (m *mockDevice) Close() error                                            { return nil }
func (m *mockDevice) Snapshot(context.Context, string) (device.Device, error) { return m, nil }
func (m *mockDevice) Cleanup(context.Context) error                           { return nil }
func (m *mockDevice) Identity(context.Context) (device.DeviceIdentity, error) {
	return device.DeviceIdentity{}, nil
}
func (m *mockDevice) AppendWAL(device.Range) error              { return nil }
func (m *mockDevice) RecoverWAL(func(device.Range) error) error { return nil }

func minimalStream(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := common.WriteHandshake(&buf, common.Handshake{Compress: "none", Checksum: true, CRC32C: true, Digests: []string{"sha256"}, Digest: "sha256"}); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	return buf.Bytes()
}

func TestResumeVerifySuccess(t *testing.T) {
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
	var st unix.Stat_t
	if err := unix.Stat(dest, &st); err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	idx, err := manifestpkg.Create(man, "uuid", uint64(len(data)), 0, uint32(unix.Major(uint64(st.Rdev))), uint32(unix.Minor(uint64(st.Rdev))), uint32(blockSize), 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	dig := blake3.Sum256(first)
	xx := hashutil.SumXXH3(first)
	if err := idx.Set(0, uint32(blockSize), 0, xx, dig); err != nil {
		t.Fatalf("manifest set: %v", err)
	}
	dig = blake3.Sum256(second)
	xx = hashutil.SumXXH3(second)
	if err := idx.Set(uint64(blockSize), uint32(blockSize), 0, xx, dig); err != nil {
		t.Fatalf("manifest set: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("manifest close: %v", err)
	}
	resume := filepath.Join(dir, "resume.state")
	w, _, err := transfer.OpenWAL(resume+".wal", uint64(len(data)), "uuid", 0, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(transfer.Range{Start: 0, End: uint64(blockSize)}); err != nil {
		t.Fatalf("append wal: %v", err)
	}
	if err := w.Append(transfer.Range{Start: uint64(blockSize), End: uint64(len(data))}); err != nil {
		t.Fatalf("append wal: %v", err)
	}
	w.Close()
	var firstDigest [32]byte
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
	tr := transfer.NewTransfer(zap.NewNop(), nil, info)
	cfg := &config.Config{
		BlockSize:         blockSize,
		ManifestPath:      man,
		ResumeState:       resume,
		Compress:          "none",
		ChecksumAlgorithm: "sha256",
		VerifyLevel:       "inline",
		MaxRetries:        1,
	}
	if err := tr.ProcessDumpData(context.Background(), cfg, bytes.NewReader(minimalStream(t)), dest); err != nil {
		t.Fatalf("verification failed: %v", err)
	}
}

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
	var st2 unix.Stat_t
	if err := unix.Stat(dest, &st2); err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	idx, err := manifestpkg.Create(man, "uuid", uint64(len(data)), 0, uint32(unix.Major(uint64(st2.Rdev))), uint32(unix.Minor(uint64(st2.Rdev))), uint32(blockSize), 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	dig := blake3.Sum256(first)
	xx := hashutil.SumXXH3(first)
	if err := idx.Set(0, uint32(blockSize), 0, xx, dig); err != nil {
		t.Fatalf("manifest set: %v", err)
	}
	dig = blake3.Sum256(second)
	xx = hashutil.SumXXH3(second)
	if err := idx.Set(uint64(blockSize), uint32(blockSize), 0, xx, dig); err != nil {
		t.Fatalf("manifest set: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("manifest close: %v", err)
	}
	resume := filepath.Join(dir, "resume.state")
	w, _, err := transfer.OpenWAL(resume+".wal", uint64(len(data)), "uuid", 0, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(transfer.Range{Start: 0, End: uint64(blockSize)}); err != nil {
		t.Fatalf("append wal: %v", err)
	}
	if err := w.Append(transfer.Range{Start: uint64(blockSize), End: uint64(len(data))}); err != nil {
		t.Fatalf("append wal: %v", err)
	}
	w.Close()
	corrupt := bytes.Repeat([]byte{3}, blockSize)
	f, err := os.OpenFile(dest, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open dest: %v", err)
	}
	if _, err := f.WriteAt(corrupt, int64(blockSize)); err != nil {
		t.Fatalf("corrupt dest: %v", err)
	}
	f.Close()
	var firstDigest [32]byte
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
	tr := transfer.NewTransfer(zap.NewNop(), nil, info)
	cfg := &config.Config{
		BlockSize:         blockSize,
		ManifestPath:      man,
		ResumeState:       resume,
		Compress:          "none",
		ChecksumAlgorithm: "sha256",
		VerifyLevel:       "inline",
		MaxRetries:        1,
	}
	err = tr.ProcessDumpData(context.Background(), cfg, bytes.NewReader(minimalStream(t)), dest)
	if err == nil {
		t.Fatalf("expected verification failure")
	}
}
