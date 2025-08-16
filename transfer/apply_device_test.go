package transfer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"lvmsync_go/common"
	"lvmsync_go/config"
	"lvmsync_go/device"

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
	prevLVM := device.SetLVMUUIDFunc(func(context.Context, string) (string, error) { return "", errors.New("no lvm") })
	defer device.SetLVMUUIDFunc(prevLVM)
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "actual", nil })
	defer device.SetUUIDFunc(prevUUID)
	prevMount := device.SetMountFunc(func(string) (bool, error) { return false, nil })
	defer device.SetMountFunc(prevMount)

	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{})
	cfg := &config.Config{BlockSize: 1024, Compress: "none", MaxRetries: 1, DeviceUUID: "expected", DedupStrategy: "none", VerifyChecksum: true}

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
	prevLVM := device.SetLVMUUIDFunc(func(context.Context, string) (string, error) { return "", errors.New("no lvm") })
	defer device.SetLVMUUIDFunc(prevLVM)
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "id", nil })
	defer device.SetUUIDFunc(prevUUID)
	prevMount := device.SetMountFunc(func(string) (bool, error) { return true, nil })
	defer device.SetMountFunc(prevMount)

	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{})
	cfg := &config.Config{BlockSize: 1024, Compress: "none", MaxRetries: 1, DeviceUUID: "id", DedupStrategy: "none", VerifyChecksum: true}

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
	prevLVM := device.SetLVMUUIDFunc(func(context.Context, string) (string, error) { return "", errors.New("no lvm") })
	defer device.SetLVMUUIDFunc(prevLVM)
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "actual", nil })
	defer device.SetUUIDFunc(prevUUID)
	prevMount := device.SetMountFunc(func(string) (bool, error) { return false, nil })
	defer device.SetMountFunc(prevMount)

	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{})
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
	prevLVM := device.SetLVMUUIDFunc(func(context.Context, string) (string, error) { return "", errors.New("no lvm") })
	defer device.SetLVMUUIDFunc(prevLVM)
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "id", nil })
	defer device.SetUUIDFunc(prevUUID)
	prevMount := device.SetMountFunc(func(string) (bool, error) { return true, nil })
	defer device.SetMountFunc(prevMount)

	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{})
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
	prevLVM := device.SetLVMUUIDFunc(func(context.Context, string) (string, error) { return "", errors.New("no lvm") })
	defer device.SetLVMUUIDFunc(prevLVM)
	prevUUID := device.SetUUIDFunc(func(ctx context.Context, _ string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	defer device.SetUUIDFunc(prevUUID)
	prevMount := device.SetMountFunc(func(string) (bool, error) { return false, nil })
	defer device.SetMountFunc(prevMount)

	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{})
	cfg := &config.Config{BlockSize: 1024, Compress: "none", MaxRetries: 1, DeviceUUID: "id", DedupStrategy: "none", VerifyChecksum: true}

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
