package device

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	dev, err := Detect(f.Name())
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
	dev, err := Detect(loop)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*RawDevice); !ok {
		t.Fatalf("expected RawDevice, got %T", dev)
	}
	dev.Close()
}

func TestDetectLVM(t *testing.T) {
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

	dev, err := Detect(lvPath)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*LVMDevice); !ok {
		t.Fatalf("expected LVMDevice, got %T", dev)
	}
	dev.Close()
}
