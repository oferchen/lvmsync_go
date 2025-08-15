package device

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func setupLoop(t *testing.T, size int64) (string, func()) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "loopback")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatalf("truncate: %v", err)
	}
	f.Close()

	out, err := exec.Command("losetup", "--show", "-f", f.Name()).Output()
	if err != nil {
		t.Skipf("losetup: %v", err)
	}
	loop := strings.TrimSpace(string(out))
	loop = filepath.Clean(loop)
	cleanup := func() { exec.Command("losetup", "-d", loop).Run() }
	return loop, cleanup
}

func TestDetectFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	dev, err := Detect(context.Background(), f.Name(), true, "", "", "", "", 0, 0, logger)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*FileDevice); !ok {
		t.Fatalf("expected FileDevice, got %T", dev)
	}
	dev.Close()

	entries := logs.FilterMessage("detect device success").All()
	found := false
	for _, e := range entries {
		if e.ContextMap()["device_type"] == "file" && e.ContextMap()["path"] == f.Name() {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected log entry for file detection")
	}
}

func TestDetectFileSymlink(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	link := filepath.Join(t.TempDir(), "filelink")
	if err := os.Symlink(f.Name(), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	dev, err := Detect(context.Background(), link, true, "", "", "", "", 0, 0, nil)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*FileDevice); !ok {
		t.Fatalf("expected FileDevice, got %T", dev)
	}
	dev.Close()
}

func TestDetectRaw(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	dev, err := Detect(context.Background(), loop, true, "", "", "", "", 0, 0, logger)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*RawDevice); !ok {
		t.Fatalf("expected RawDevice, got %T", dev)
	}
	dev.Close()

	// Expect LVM failure debug and Raw success info.
	lvmFail := false
	rawSuccess := false
	for _, e := range logs.All() {
		switch e.Message {
		case "detect device failed":
			if e.ContextMap()["device_type"] == "lvm" {
				lvmFail = true
			}
		case "detect device success":
			if e.ContextMap()["device_type"] == "raw" {
				rawSuccess = true
			}
		}
	}
	if !lvmFail || !rawSuccess {
		t.Fatalf("expected lvm fail and raw success logs, got %v", logs.All())
	}
}

func TestDetectRawSymlink(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	link := filepath.Join(t.TempDir(), "rawlink")
	if err := os.Symlink(loop, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	dev, err := Detect(context.Background(), link, true, "", "", "", "", 0, 0, nil)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*RawDevice); !ok {
		t.Fatalf("expected RawDevice, got %T", dev)
	}
	dev.Close()
}

func TestDetectLVMSymlink(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	vgDir := filepath.Join("/dev", "testvg")
	lvPath := filepath.Join(vgDir, "testlv")
	os.MkdirAll(vgDir, 0o755)
	os.Symlink(loop, lvPath)
	defer os.Remove(lvPath)
	defer os.Remove(vgDir)

	dev, err := Detect(context.Background(), lvPath, true, "", "", "", "", 0, 0, nil)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*RawDevice); !ok {
		t.Fatalf("expected RawDevice, got %T", dev)
	}
	dev.Close()
}
