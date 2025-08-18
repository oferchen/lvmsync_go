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

	"lvmsync_go/common"
	"lvmsync_go/device"
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

func minimalStream(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := common.WriteHandshake(&buf, common.Handshake{Compress: "none", Checksum: true, CRC32C: true, Digests: []string{"sha256"}, Digest: "sha256"}); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	return buf.Bytes()
}

func buildManifest(t *testing.T, devPath, manPath, uuid string, size uint64) {
	t.Helper()
	detect := func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error) {
		return &mockDevice{path: devPath, size: size, blockSize: 4}, nil
	}
	info := device.NewInfoWithDeps(func(context.Context, string) (string, error) { return uuid, nil }, nil, nil, nil, nil)
	if err := manifestpkg.Rebuild(context.Background(), devPath, manPath, zap.NewNop(), 0, false, 0, 0, 0, 0, manifestpkg.WithDetectDevice(detect), manifestpkg.WithDeviceInfo(info)); err != nil {
		t.Fatalf("rebuild: %v", err)
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
	buildManifest(t, dest, man, "uuid", uint64(len(data)))
	resume := filepath.Join(dir, "resume.state")
	w, _, err := transfer.OpenWAL(resume+".wal", uint64(len(data)), "uuid", 0)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(transfer.Range{Start: 0, End: uint64(len(data))}); err != nil {
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
	tr := transfer.NewTransfer(zap.NewNop(), nil, info)
	cfg := &config.Config{
		BlockSize:         blockSize,
		ManifestPath:      man,
		ResumeState:       resume,
		Compress:          "none",
		ChecksumAlgorithm: "sha256",
		VerifyLevel:       "full",
		MaxRetries:        1,
	}
	err = tr.ProcessDumpData(context.Background(), cfg, bytes.NewReader(minimalStream(t)), dest)
	if err == nil {
		t.Fatalf("expected verification failure")
	}
}
