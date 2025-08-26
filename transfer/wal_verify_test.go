package transfer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	manifestpkg "lvmsync_go/manifest"
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
	return device.DeviceIdentity{SizeBytes: m.size}, nil
}
func (m *mockDevice) AppendWAL(r device.Range) error               { return nil }
func (m *mockDevice) RecoverWAL(fn func(device.Range) error) error { return nil }

func buildManifest(t *testing.T, devPath, manPath, uuid string, size uint64) {
	t.Helper()
	detect := func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error) {
		return &mockDevice{path: devPath, size: size, blockSize: 4}, nil
	}
	info := device.NewInfoWithDeps(func(context.Context, string) (string, error) { return uuid, nil }, nil, nil, nil, nil)
	if err := manifestpkg.Rebuild(context.Background(), devPath, manPath, zap.NewNop(), 0, true, 0, 0, 0, 0, manifestpkg.WithDetectDevice(detect), manifestpkg.WithDeviceInfo(info)); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
}

func TestWALVerifySuccess(t *testing.T) {
	dir := t.TempDir()
	dev := filepath.Join(dir, "dev")
	data := []byte("abcd")
	if err := os.WriteFile(dev, data, 0o600); err != nil {
		t.Fatalf("write dev: %v", err)
	}
	man := filepath.Join(dir, "man")
	buildManifest(t, dev, man, "uuid", uint64(len(data)))
	walPath := filepath.Join(dir, "wal")
	id := device.DeviceIdentity{SizeBytes: uint64(len(data)), KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000000", FSUUID: "uuid"}
	w, _, err := OpenWAL(walPath, id, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: uint64(len(data))}); err != nil {
		t.Fatalf("append: %v", err)
	}
	w.Close()
	cfg := &config.Config{BlockSize: len(data), ManifestPath: man}
	f, err := os.Open(dev)
	if err != nil {
		t.Fatalf("open dev: %v", err)
	}
	defer f.Close()
	if err := verifyWAL(cfg, f, []Range{{Start: 0, End: uint64(len(data))}}, zap.NewNop()); err != nil {
		t.Fatalf("verifyWAL: %v", err)
	}
}

func TestWALVerifyMismatch(t *testing.T) {
	dir := t.TempDir()
	dev := filepath.Join(dir, "dev")
	data := []byte("abcd")
	if err := os.WriteFile(dev, data, 0o600); err != nil {
		t.Fatalf("write dev: %v", err)
	}
	man := filepath.Join(dir, "man")
	buildManifest(t, dev, man, "uuid", uint64(len(data)))
	walPath := filepath.Join(dir, "wal")
	id := device.DeviceIdentity{SizeBytes: uint64(len(data)), KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000000", FSUUID: "uuid"}
	w, _, err := OpenWAL(walPath, id, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: uint64(len(data))}); err != nil {
		t.Fatalf("append: %v", err)
	}
	w.Close()
	// Corrupt device
	if err := os.WriteFile(dev, []byte("wxyz"), 0o600); err != nil {
		t.Fatalf("corrupt dev: %v", err)
	}
	cfg := &config.Config{BlockSize: len(data), ManifestPath: man}
	f, err := os.Open(dev)
	if err != nil {
		t.Fatalf("open dev: %v", err)
	}
	defer f.Close()
	if err := verifyWAL(cfg, f, []Range{{Start: 0, End: uint64(len(data))}}, zap.NewNop()); err == nil {
		t.Fatalf("expected verifyWAL error")
	}
}
