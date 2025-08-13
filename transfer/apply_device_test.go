package transfer

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"lvmsync_go/common"
	"lvmsync_go/config"
	"lvmsync_go/device"

	"go.uber.org/zap"
)

func minimalStream(t *testing.T) []byte {
	var buf bytes.Buffer
	if err := common.WriteHandshake(&buf, common.Handshake{Compress: "none", Checksum: true}); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	return buf.Bytes()
}

func TestProcessDumpDataUUIDMismatch(t *testing.T) {
	prevUUID := device.SetUUIDFunc(func(string) (string, error) { return "actual", nil })
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
	if err := f.Truncate(1024); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f.Close()

	err = tr.ProcessDumpData(cfg, bytes.NewReader(minimalStream(t)), dest)
	if err == nil {
		t.Fatalf("expected error for uuid mismatch")
	}
}

func TestProcessDumpDataMountedDevice(t *testing.T) {
	prevUUID := device.SetUUIDFunc(func(string) (string, error) { return "id", nil })
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
	if err := f.Truncate(1024); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f.Close()

	err = tr.ProcessDumpData(cfg, bytes.NewReader(minimalStream(t)), dest)
	if err == nil {
		t.Fatalf("expected error for mounted device")
	}

	cfg.Force = true
	err = tr.ProcessDumpData(cfg, bytes.NewReader(minimalStream(t)), dest)
	if err != nil {
		t.Fatalf("unexpected error with force: %v", err)
	}
}

func TestApplyDataUUIDMismatch(t *testing.T) {
	prevUUID := device.SetUUIDFunc(func(string) (string, error) { return "actual", nil })
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
	if err := f.Truncate(1024); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f.Close()

	err = tr.applyData(cfg, bytes.NewReader(minimalStream(t)), dest)
	if err == nil {
		t.Fatalf("expected error for uuid mismatch")
	}
}

func TestApplyDataMountedDevice(t *testing.T) {
	prevUUID := device.SetUUIDFunc(func(string) (string, error) { return "id", nil })
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
	if err := f.Truncate(1024); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f.Close()

	err = tr.applyData(cfg, bytes.NewReader(minimalStream(t)), dest)
	if err == nil {
		t.Fatalf("expected error for mounted device")
	}
}
