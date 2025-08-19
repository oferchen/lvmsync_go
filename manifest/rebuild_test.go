package manifest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"

	"lvmsync_go/device"
)

func TestRegenerateMissing(t *testing.T) {
	dir := t.TempDir()
	dev := filepath.Join(dir, "dev")
	data := []byte("aaaabbbb")
	if err := os.WriteFile(dev, data, 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}
	man := filepath.Join(dir, "dev.man")
	detect := func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error) {
		return &mockDevice{path: dev, size: uint64(len(data)), blockSize: 4}, nil
	}
	info := device.NewInfoWithDeps(func(context.Context, string) (string, error) { return "uuid", nil }, nil, nil, nil, nil)
	if err := Regenerate(context.Background(), dev, man, zap.NewNop(), 0, false, 0, 0, 0, 0, WithDetectDevice(detect), WithDeviceInfo(info)); err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	idx, err := Open(man)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer idx.Close()
	off, length, _, xx, dig, err := idx.Entry(0)
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	if off != 0 || length != 4 {
		t.Fatalf("unexpected entry metadata")
	}
	if xx != xxh3.Hash(data[:4]) || dig != blake3.Sum256(data[:4]) {
		t.Fatalf("manifest not rebuilt correctly")
	}
}

func TestRegenerateStale(t *testing.T) {
	dir := t.TempDir()
	dev := filepath.Join(dir, "dev")
	orig := []byte("aaaabbbb")
	if err := os.WriteFile(dev, orig, 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}
	man := filepath.Join(dir, "dev.man")
	detect := func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error) {
		return &mockDevice{path: dev, size: uint64(len(orig)), blockSize: 4}, nil
	}
	info := device.NewInfoWithDeps(func(context.Context, string) (string, error) { return "uuid", nil }, nil, nil, nil, nil)
	if err := Rebuild(context.Background(), dev, man, zap.NewNop(), 0, false, 0, 0, 0, 0, WithDetectDevice(detect), WithDeviceInfo(info)); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	// modify device so manifest becomes stale
	updated := []byte("ccccdddd")
	if err := os.WriteFile(dev, updated, 0o600); err != nil {
		t.Fatalf("update device: %v", err)
	}
	if err := Regenerate(context.Background(), dev, man, zap.NewNop(), 0, false, 0, 0, 0, 0, WithDetectDevice(detect), WithDeviceInfo(info)); err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	idx, err := Open(man)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer idx.Close()
	_, _, _, xx, dig, err := idx.Entry(0)
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	if xx != xxh3.Hash(updated[:4]) || dig != blake3.Sum256(updated[:4]) {
		t.Fatalf("manifest not updated")
	}
}

func TestRegenerateExisting(t *testing.T) {
	dir := t.TempDir()
	dev := filepath.Join(dir, "dev")
	data := []byte("aaaabbbb")
	if err := os.WriteFile(dev, data, 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}
	man := filepath.Join(dir, "dev.man")
	detect := func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error) {
		return &mockDevice{path: dev, size: uint64(len(data)), blockSize: 4}, nil
	}
	info := device.NewInfoWithDeps(func(context.Context, string) (string, error) { return "uuid", nil }, nil, nil, nil, nil)
	if err := Rebuild(context.Background(), dev, man, zap.NewNop(), 0, false, 0, 0, 0, 0, WithDetectDevice(detect), WithDeviceInfo(info)); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	before, err := os.Stat(man)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := Regenerate(context.Background(), dev, man, zap.NewNop(), 0, false, 0, 0, 0, 0, WithDetectDevice(detect), WithDeviceInfo(info)); err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	after, err := os.Stat(man)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("manifest should not change when up to date")
	}
}

func TestRegenerateSizeMismatch(t *testing.T) {
	dir := t.TempDir()
	dev := filepath.Join(dir, "dev")
	orig := []byte("aaaabbbb")
	if err := os.WriteFile(dev, orig, 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}
	man := filepath.Join(dir, "dev.man")
	size := uint64(len(orig))
	detect := func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error) {
		return &mockDevice{path: dev, size: size, blockSize: 4}, nil
	}
	info := device.NewInfoWithDeps(func(context.Context, string) (string, error) { return "uuid", nil }, nil, nil, nil, nil)
	if err := Rebuild(context.Background(), dev, man, zap.NewNop(), 0, false, 0, 0, 0, 0, WithDetectDevice(detect), WithDeviceInfo(info)); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	before, err := os.Stat(man)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	enlarged := []byte("aaaabbbbcccc")
	if err := os.WriteFile(dev, enlarged, 0o600); err != nil {
		t.Fatalf("enlarge device: %v", err)
	}
	size = uint64(len(enlarged))
	if err := Regenerate(context.Background(), dev, man, zap.NewNop(), 0, false, 0, 0, 0, 0, WithDetectDevice(detect), WithDeviceInfo(info)); err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	after, err := os.Stat(man)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("manifest not rebuilt on size mismatch")
	}
	idx, err := Open(man)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer idx.Close()
	if idx.hdr.SizeBytes != uint64(len(enlarged)) || idx.hdr.ChunkCount != uint64(len(enlarged))/4 {
		t.Fatalf("unexpected header: size %d chunks %d", idx.hdr.SizeBytes, idx.hdr.ChunkCount)
	}
	off, length, _, xx, dig, err := idx.Entry(2)
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	if off != 8 || length != 4 {
		t.Fatalf("unexpected entry metadata")
	}
	if xx != xxh3.Hash(enlarged[8:12]) || dig != blake3.Sum256(enlarged[8:12]) {
		t.Fatalf("manifest not rebuilt correctly")
	}
}
