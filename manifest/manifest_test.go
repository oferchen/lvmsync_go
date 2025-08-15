package manifest

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/device"
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

func TestIndexCRUD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.man")
	idx, err := Create(path, "dev", 8192, 4096)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	data := make([]byte, 4096)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	xx := xxh3.Hash(data)
	b3 := blake3.Sum256(data)
	if err := idx.Set(0, 4096, xx, b3); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !idx.Match(0, 4096, xx, func() [32]byte { return b3 }) {
		t.Fatalf("Match failed")
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	idx2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	off, length, xx2, b32, err := idx2.Entry(0)
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if off != 0 || length != 4096 || xx2 != xx || b32 != b3 {
		t.Fatalf("entry mismatch")
	}
	if err := idx2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}
}

func TestDeviceIDLength(t *testing.T) {
	dir := t.TempDir()
	good := strings.Repeat("a", 64)
	goodPath := filepath.Join(dir, "good.man")
	if _, err := Create(goodPath, good, 4096, 4096); err != nil {
		t.Fatalf("expected success for 64-byte id: %v", err)
	}
	bad := strings.Repeat("b", 65)
	badPath := filepath.Join(dir, "bad.man")
	if _, err := Create(badPath, bad, 4096, 4096); err == nil {
		t.Fatalf("expected error for id >64 bytes")
	}
}

func TestUpgrade(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.man")
	idx, err := Create(path, "dev", 4096, 4096)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	idx.hdr.Version = 0
	idx.hdr.MAC = headerMAC(&idx.hdr)
	idx.writeHeader()
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := Open(path); !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("expected version mismatch, got %v", err)
	}
	up, err := Upgrade(path)
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if up.hdr.Version != Version {
		t.Fatalf("version not upgraded: %d", up.hdr.Version)
	}
	if err := up.Close(); err != nil {
		t.Fatalf("close upgraded: %v", err)
	}
	idx2, err := Open(path)
	if err != nil {
		t.Fatalf("Open after upgrade: %v", err)
	}
	if idx2.hdr.Version != Version {
		t.Fatalf("version mismatch after upgrade: %d", idx2.hdr.Version)
	}
	if err := idx2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}
}

func TestCreateCloseHook(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hook.man")
	called := 0
	idx, err := Create(path, "dev", 4096, 4096, WithCloseHook(func() error { called++; return nil }))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected close hook called once, got %d", called)
	}
}

func TestOpenCloseHook(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hook.man")
	idx, err := Create(path, "dev", 4096, 4096)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	called := 0
	idx, err = Open(path, WithCloseHook(func() error { called++; return nil }))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected close hook called once, got %d", called)
	}
}

func TestUpgradeCloseHook(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hook.man")
	idx, err := Create(path, "dev", 4096, 4096)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	idx.hdr.Version = 0
	idx.hdr.MAC = headerMAC(&idx.hdr)
	idx.writeHeader()
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	called := 0
	up, err := Upgrade(path, WithCloseHook(func() error { called++; return nil }))
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if err := up.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected close hook called once, got %d", called)
	}
}

func TestMatchXXHShortcut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shortcut.man")
	idx, err := Create(path, "dev", 4096, 4096)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	data := make([]byte, 4096)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	xx := xxh3.Hash(data)
	b3 := blake3.Sum256(data)
	if err := idx.Set(0, 4096, xx, b3); err != nil {
		t.Fatalf("Set: %v", err)
	}

	other := make([]byte, 4096)
	if _, err := rand.Read(other); err != nil {
		t.Fatalf("rand other: %v", err)
	}
	xxOther := xxh3.Hash(other)
	called := false
	if idx.Match(0, 4096, xxOther, func() [32]byte {
		called = true
		return blake3.Sum256(other)
	}) {
		t.Fatalf("unexpected match")
	}
	if called {
		t.Fatalf("digest function called despite XXH3 mismatch")
	}
}

func TestIndexLargeOffsets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.man")
	const blockSize = uint32(1 << 31) // 2 GiB
	const chunks = uint64(3)
	size := uint64(blockSize) * chunks
	idx, err := Create(path, "dev", size, blockSize)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	data := []byte("data")
	xx := xxh3.Hash(data)
	b3 := blake3.Sum256(data)
	offset := uint64(blockSize) * 2 // 4 GiB > int32
	if err := idx.Set(offset, uint32(len(data)), xx, b3); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !idx.Match(offset, uint32(len(data)), xx, func() [32]byte { return b3 }) {
		t.Fatalf("Match failed for large offset")
	}
	off, length, xx2, b32, err := idx.Entry(2)
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if off != offset || length != uint32(len(data)) || xx2 != xx || b32 != b3 {
		t.Fatalf("entry mismatch for large offset")
	}
	if idx.ChunkCount() != chunks {
		t.Fatalf("chunk count mismatch: got %d want %d", idx.ChunkCount(), chunks)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestIndexOutOfRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oor.man")
	idx, err := Create(path, "dev", 4096, 4096)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer idx.Close()
	if err := idx.Set(4096, 4096, 0, [32]byte{}); err == nil {
		t.Fatalf("expected error for Set out of range")
	}
	if _, _, _, _, err := idx.Entry(1); err == nil {
		t.Fatalf("expected error for Entry out of range")
	}
}

func TestRebuild(t *testing.T) {
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "dev-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	data1 := make([]byte, 4096)
	data2 := make([]byte, 4096)
	if _, err := rand.Read(data1); err != nil {
		t.Fatalf("rand1: %v", err)
	}
	if _, err := rand.Read(data2); err != nil {
		t.Fatalf("rand2: %v", err)
	}
	if _, err := file.Write(append(data1, data2...)); err != nil {
		t.Fatalf("write: %v", err)
	}
	file.Close()
	prev := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid-test", nil })
	defer device.SetUUIDFunc(prev)
	manPath := filepath.Join(dir, "rebuild.man")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Rebuild(ctx, file.Name(), manPath, zap.NewNop(), 0, false); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	idx, err := Open(manPath)
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	if idx.hdr.BlockSize != 4096 || idx.hdr.ChunkCount != 2 || idx.hdr.SizeBytes != 8192 {
		t.Fatalf("header mismatch: %+v", idx.hdr)
	}
	if id := string(bytes.TrimRight(idx.hdr.DeviceID[:], "\x00")); id != "uuid-test" {
		t.Fatalf("device id mismatch: %s", id)
	}
	_, l1, xx1, b31, err := idx.Entry(0)
	if err != nil {
		t.Fatalf("Entry0: %v", err)
	}
	if l1 != 4096 || xx1 != xxh3.Hash(data1) || b31 != blake3.Sum256(data1) {
		t.Fatalf("chunk1 mismatch")
	}
	_, l2, xx2, b32, err := idx.Entry(1)
	if err != nil {
		t.Fatalf("Entry1: %v", err)
	}
	if l2 != 4096 || xx2 != xxh3.Hash(data2) || b32 != blake3.Sum256(data2) {
		t.Fatalf("chunk2 mismatch")
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close manifest: %v", err)
	}
}

func TestRebuildCloseHook(t *testing.T) {
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "dev-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	data := make([]byte, 4096)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	file.Close()
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid-test", nil })
	defer device.SetUUIDFunc(prevUUID)
	manPath := filepath.Join(dir, "closeonce.man")
	count := 0
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Rebuild(ctx, file.Name(), manPath, zap.NewNop(), 0, false, WithCloseHook(func() error { count++; return nil })); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected close called once, got %d", count)
	}
}

func TestRebuildCloseError(t *testing.T) {
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "dev-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	data := make([]byte, 4096)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	file.Close()
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid-test", nil })
	defer device.SetUUIDFunc(prevUUID)
	manPath := filepath.Join(dir, "closeerr.man")
	hookErr := errors.New("close fail")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Rebuild(ctx, file.Name(), manPath, zap.NewNop(), 0, false, WithCloseHook(func() error { return hookErr })); err == nil || !strings.Contains(err.Error(), "close fail") {
		t.Fatalf("expected close error, got %v", err)
	}
}

func TestRebuildOptionApplication(t *testing.T) {
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "dev-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	data := make([]byte, 4096)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	file.Close()

	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid-test", nil })
	defer device.SetUUIDFunc(prevUUID)

	calledDetect := false
	detect := func(context.Context, string, *zap.Logger) (device.Device, error) {
		calledDetect = true
		return &mockDevice{path: file.Name(), size: uint64(len(data)), blockSize: 4096}, nil
	}
	calledHook := false
	hook := func() error { calledHook = true; return nil }

	manPath := filepath.Join(dir, "options.man")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Rebuild(ctx, file.Name(), manPath, zap.NewNop(), 0, false, WithDetectDevice(detect), WithCloseHook(hook)); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if !calledDetect {
		t.Fatalf("detect not called")
	}
	if !calledHook {
		t.Fatalf("close hook not called")
	}
}

func TestRebuildNonDefaultBlockSize(t *testing.T) {
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "dev-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	bs := 2048
	data1 := make([]byte, bs)
	data2 := make([]byte, bs)
	if _, err := rand.Read(data1); err != nil {
		t.Fatalf("rand1: %v", err)
	}
	if _, err := rand.Read(data2); err != nil {
		t.Fatalf("rand2: %v", err)
	}
	if _, err := file.Write(append(data1, data2...)); err != nil {
		t.Fatalf("write: %v", err)
	}
	file.Close()
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid-bs", nil })
	defer device.SetUUIDFunc(prevUUID)
	detect := func(context.Context, string, *zap.Logger) (device.Device, error) {
		return &mockDevice{path: file.Name(), size: uint64(2 * bs), blockSize: uint64(bs)}, nil
	}
	manPath := filepath.Join(dir, "rebuildbs.man")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Rebuild(ctx, file.Name(), manPath, zap.NewNop(), 0, false, WithDetectDevice(detect)); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	idx, err := Open(manPath)
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	if idx.hdr.BlockSize != uint32(bs) || idx.hdr.ChunkCount != 2 || idx.hdr.SizeBytes != uint64(2*bs) {
		t.Fatalf("header mismatch: %+v", idx.hdr)
	}
	_, l1, xx1, b31, err := idx.Entry(0)
	if err != nil {
		t.Fatalf("Entry0: %v", err)
	}
	if l1 != uint32(bs) || xx1 != xxh3.Hash(data1) || b31 != blake3.Sum256(data1) {
		t.Fatalf("chunk1 mismatch")
	}
	_, l2, xx2, b32, err := idx.Entry(1)
	if err != nil {
		t.Fatalf("Entry1: %v", err)
	}
	if l2 != uint32(bs) || xx2 != xxh3.Hash(data2) || b32 != blake3.Sum256(data2) {
		t.Fatalf("chunk2 mismatch")
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close manifest: %v", err)
	}
}

func TestRebuildLogsProgress(t *testing.T) {
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "dev-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	data := make([]byte, 8192)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	file.Close()
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid-test", nil })
	defer device.SetUUIDFunc(prevUUID)
	manPath := filepath.Join(dir, "progress.man")
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Rebuild(ctx, file.Name(), manPath, logger, 0, false); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	entries := logs.FilterMessage("rebuild progress").All()
	if len(entries) != 2 {
		t.Fatalf("expected 2 progress logs, got %d", len(entries))
	}
	if off, ok := entries[0].ContextMap()["offset_bytes"].(uint64); !ok || off != 4096 {
		t.Fatalf("unexpected first offset %v", entries[0].ContextMap()["offset_bytes"])
	}
	if _, ok := entries[0].ContextMap()["duration_ms"]; !ok {
		t.Fatalf("missing duration_ms field")
	}
}

func TestRebuildLogsCompletion(t *testing.T) {
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "dev-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	data := make([]byte, 8192)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	file.Close()
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid-test", nil })
	defer device.SetUUIDFunc(prevUUID)
	manPath := filepath.Join(dir, "complete.man")
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Rebuild(ctx, file.Name(), manPath, logger, 0, false); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	entries := logs.FilterMessage("rebuild_complete").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 completion log, got %d", len(entries))
	}
	if sz, ok := entries[0].ContextMap()["size_bytes"].(uint64); !ok || sz != uint64(len(data)) {
		t.Fatalf("unexpected size_bytes %v", entries[0].ContextMap()["size_bytes"])
	}
	if _, ok := entries[0].ContextMap()["duration_ms"]; !ok {
		t.Fatalf("missing duration_ms field")
	}
}

func TestRebuildCanceledContext(t *testing.T) {
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "dev-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	size := 1 << 20 // 1 MiB
	if err := file.Truncate(int64(size)); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	file.Close()
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid-test", nil })
	defer device.SetUUIDFunc(prevUUID)
	detect := func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error) {
		return &mockDevice{path: file.Name(), size: uint64(size), blockSize: 1}, nil
	}
	manPath := filepath.Join(dir, "cancel.man")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	if err := Rebuild(ctx, file.Name(), manPath, zap.NewNop(), 0, false, WithDetectDevice(detect)); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRebuildMounted(t *testing.T) {
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "dev-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := file.Write(make([]byte, 4096)); err != nil {
		t.Fatalf("write: %v", err)
	}
	file.Close()
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid-test", nil })
	defer device.SetUUIDFunc(prevUUID)
	cases := []struct {
		name    string
		mounted bool
		allow   bool
		wantErr bool
	}{
		{"mounted_disallowed", true, false, true},
		{"mounted_allowed", true, true, false},
		{"unmounted", false, false, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			prevMount := device.SetMountFunc(func(string) (bool, error) { return tt.mounted, nil })
			t.Cleanup(func() { device.SetMountFunc(prevMount) })
			manPath := filepath.Join(dir, tt.name+".man")
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := Rebuild(ctx, file.Name(), manPath, zap.NewNop(), 0, tt.allow)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
