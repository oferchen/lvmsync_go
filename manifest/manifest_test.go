package manifest

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"

	"lvmsync_go/device"
)

type mockDevice struct {
	path      string
	size      uint64
	blockSize uint64
}

func (m *mockDevice) Path() string      { return m.path }
func (m *mockDevice) SizeBytes() uint64 { return m.size }
func (m *mockDevice) BlockSize() uint64 { return m.blockSize }
func (m *mockDevice) Close() error      { return nil }

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
	idx.Set(0, 4096, xx, b3)
	if !idx.Match(0, 4096, b3) {
		t.Fatalf("Match failed")
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	idx2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	off, length, xx2, b32 := idx2.Entry(0)
	if off != 0 || length != 4096 || xx2 != xx || b32 != b3 {
		t.Fatalf("entry mismatch")
	}
	if err := idx2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
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
	if err := Rebuild(file.Name(), manPath); err != nil {
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
	_, l1, xx1, b31 := idx.Entry(0)
	if l1 != 4096 || xx1 != xxh3.Hash(data1) || b31 != blake3.Sum256(data1) {
		t.Fatalf("chunk1 mismatch")
	}
	_, l2, xx2, b32 := idx.Entry(1)
	if l2 != 4096 || xx2 != xxh3.Hash(data2) || b32 != blake3.Sum256(data2) {
		t.Fatalf("chunk2 mismatch")
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close manifest: %v", err)
	}
}

func TestRebuildCloseOnce(t *testing.T) {
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
	prevHook := closeHook
	count := 0
	closeHook = func() { count++ }
	defer func() { closeHook = prevHook }()
	if err := Rebuild(file.Name(), manPath); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected close called once, got %d", count)
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
	prevDetect := detectDevice
	detectDevice = func(string) (device.Device, error) {
		return &mockDevice{path: file.Name(), size: uint64(2 * bs), blockSize: uint64(bs)}, nil
	}
	defer func() { detectDevice = prevDetect }()
	manPath := filepath.Join(dir, "rebuildbs.man")
	if err := Rebuild(file.Name(), manPath); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	idx, err := Open(manPath)
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	if idx.hdr.BlockSize != uint32(bs) || idx.hdr.ChunkCount != 2 || idx.hdr.SizeBytes != uint64(2*bs) {
		t.Fatalf("header mismatch: %+v", idx.hdr)
	}
	_, l1, xx1, b31 := idx.Entry(0)
	if l1 != uint32(bs) || xx1 != xxh3.Hash(data1) || b31 != blake3.Sum256(data1) {
		t.Fatalf("chunk1 mismatch")
	}
	_, l2, xx2, b32 := idx.Entry(1)
	if l2 != uint32(bs) || xx2 != xxh3.Hash(data2) || b32 != blake3.Sum256(data2) {
		t.Fatalf("chunk2 mismatch")
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close manifest: %v", err)
	}
}
