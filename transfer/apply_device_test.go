package transfer

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"lvmsync_go/common"
	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	manifestpkg "lvmsync_go/manifest"

	"go.uber.org/zap"
)

func minimalStream(t *testing.T) []byte {
	var buf bytes.Buffer
	if err := common.WriteHandshake(&buf, common.Handshake{Compress: "none", Checksum: true, CRC32C: true, Digests: []string{"sha256"}, Digest: "sha256"}); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	return buf.Bytes()
}

func TestProcessDumpDataUUIDMismatch(t *testing.T) {
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "actual", nil },
		func(context.Context, string) (string, error) { return "", errors.New("no lvm") },
		func(context.Context, string) (bool, error) { return false, nil },
		nil,
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	cfg := &config.Config{BlockSize: 1024, Compress: "none", MaxRetries: 1, DeviceUUID: "expected", DedupStrategy: "none", VerifyChecksum: true, ChecksumAlgorithm: "sha256"}

	dest := filepath.Join(t.TempDir(), "dest")
	f, err := os.Create(dest)
	if err != nil {
		t.Fatalf("create dest: %v", err)
	}
	sentinel := []byte("original")
	if _, err := f.Write(sentinel); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := f.Truncate(1024); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f.Close()
	before, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	err = tr.ProcessDumpData(context.Background(), cfg, bytes.NewReader(minimalStream(t)), dest)
	if err == nil {
		t.Fatalf("expected error for uuid mismatch")
	}
	if !strings.Contains(err.Error(), "does not match expected") {
		t.Fatalf("unexpected error: %v", err)
	}
	after, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatalf("expected no writes to destination")
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.HasPrefix(data, sentinel) {
		t.Fatalf("destination modified")
	}
}

func TestProcessDumpDataMountedDevice(t *testing.T) {
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "id", nil },
		func(context.Context, string) (string, error) { return "", errors.New("no lvm") },
		func(context.Context, string) (bool, error) { return true, nil },
		nil,
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	cfg := &config.Config{BlockSize: 1024, Compress: "none", MaxRetries: 1, DeviceUUID: "id", DedupStrategy: "none", VerifyChecksum: true, ChecksumAlgorithm: "sha256"}

	dest := filepath.Join(t.TempDir(), "dest")
	f, err := os.Create(dest)
	if err != nil {
		t.Fatalf("create dest: %v", err)
	}
	sentinel := []byte("original")
	if _, err := f.Write(sentinel); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := f.Truncate(1024); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f.Close()
	before, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	err = tr.ProcessDumpData(context.Background(), cfg, bytes.NewReader(minimalStream(t)), dest)
	if err == nil {
		t.Fatalf("expected error for mounted device")
	}
	if !strings.Contains(err.Error(), "mounted read-write") {
		t.Fatalf("unexpected error: %v", err)
	}
	after, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatalf("expected no writes to destination")
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.HasPrefix(data, sentinel) {
		t.Fatalf("destination modified")
	}

	cfg.Force = true
	err = tr.ProcessDumpData(context.Background(), cfg, bytes.NewReader(minimalStream(t)), dest)
	if err != nil {
		t.Fatalf("unexpected error with force: %v", err)
	}
	data, err = os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.HasPrefix(data, sentinel) {
		t.Fatalf("destination modified after force")
	}
}

func TestApplyDataUUIDMismatch(t *testing.T) {
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "actual", nil },
		func(context.Context, string) (string, error) { return "", errors.New("no lvm") },
		func(context.Context, string) (bool, error) { return false, nil },
		nil,
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	cfg := &config.Config{BlockSize: 1024, Compress: "none", MaxRetries: 1, DeviceUUID: "expected"}

	dest := filepath.Join(t.TempDir(), "dest")
	f, err := os.Create(dest)
	if err != nil {
		t.Fatalf("create dest: %v", err)
	}
	sentinel := []byte("original")
	if _, err := f.Write(sentinel); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := f.Truncate(1024); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f.Close()
	before, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	err = tr.applyData(cfg, bytes.NewReader(minimalStream(t)), dest)
	if err == nil {
		t.Fatalf("expected error for uuid mismatch")
	}
	if !strings.Contains(err.Error(), "does not match expected") {
		t.Fatalf("unexpected error: %v", err)
	}
	after, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatalf("expected no writes to destination")
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.HasPrefix(data, sentinel) {
		t.Fatalf("destination modified")
	}
}

func TestApplyDataMountedDevice(t *testing.T) {
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "id", nil },
		func(context.Context, string) (string, error) { return "", errors.New("no lvm") },
		func(context.Context, string) (bool, error) { return true, nil },
		nil,
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	cfg := &config.Config{BlockSize: 1024, Compress: "none", MaxRetries: 1, DeviceUUID: "id"}

	dest := filepath.Join(t.TempDir(), "dest")
	f, err := os.Create(dest)
	if err != nil {
		t.Fatalf("create dest: %v", err)
	}
	sentinel := []byte("original")
	if _, err := f.Write(sentinel); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := f.Truncate(1024); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f.Close()
	before, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	err = tr.applyData(cfg, bytes.NewReader(minimalStream(t)), dest)
	if err == nil {
		t.Fatalf("expected error for mounted device")
	}
	if !strings.Contains(err.Error(), "mounted read-write") {
		t.Fatalf("unexpected error: %v", err)
	}
	after, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatalf("expected no writes to destination")
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.HasPrefix(data, sentinel) {
		t.Fatalf("destination modified")
	}
}
func TestProcessDumpDataCanceledContext(t *testing.T) {
	info := device.NewInfoWithDeps(
		func(ctx context.Context, _ string) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
		func(context.Context, string) (string, error) { return "", errors.New("no lvm") },
		func(context.Context, string) (bool, error) { return false, nil },
		nil,
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	cfg := &config.Config{BlockSize: 1024, Compress: "none", MaxRetries: 1, DeviceUUID: "id", DedupStrategy: "none", VerifyChecksum: true, ChecksumAlgorithm: "sha256"}

	dest := filepath.Join(t.TempDir(), "dest")
	f, err := os.Create(dest)
	if err != nil {
		t.Fatalf("create dest: %v", err)
	}
	sentinel := []byte("original")
	if _, err := f.Write(sentinel); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := f.Truncate(1024); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- tr.ProcessDumpData(ctx, cfg, bytes.NewReader(minimalStream(t)), dest) }()
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ProcessDumpData did not respect context cancellation")
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.HasPrefix(data, sentinel) {
		t.Fatalf("destination modified")
	}
}

func TestVerifyDestinationDigestMismatch(t *testing.T) {
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "id", nil },
		nil,
		func(context.Context, string) (bool, error) { return false, nil },
		func(context.Context, string, uint64) ([32]byte, error) {
			var d [32]byte
			d[0] = 1
			return d, nil
		},
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.WriteFile(dest, make([]byte, 1024), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	man := filepath.Join(t.TempDir(), "man")
	var hdr manifestpkg.Header
	hdr.Version = manifestpkg.Version
	hdr.BlockSize = 512
	hdr.SizeBytes = 1024
	hdr.ChunkCount = 2
	copy(hdr.DeviceID[:], []byte("id"))
	hdr.FirstBlockDigest[0] = 2
	hdr.MAC = manifestHeaderMAC(&hdr)
	f, err := os.Create(man)
	if err != nil {
		t.Fatalf("create manifest: %v", err)
	}
	if err := binary.Write(f, binary.LittleEndian, &hdr); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	f.Close()
	cfg := &config.Config{ManifestPath: man}
	if err := tr.verifyDestination(context.Background(), cfg, dest); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
}
