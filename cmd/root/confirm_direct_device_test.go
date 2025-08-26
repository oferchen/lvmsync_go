package root

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"lvmsync_go/internal/config"
)

// createBlockDevice returns path to a temporary block device file.
func createBlockDevice(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	devPath := filepath.Join(dir, "blkdev")
	if err := unix.Mknod(devPath, unix.S_IFBLK|0600, int(unix.Mkdev(1, 5))); err != nil {
		t.Skipf("mknod failed: %v", err)
	}
	return devPath
}

func TestConfirmDirectDeviceNonBlock(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	if err := confirmDirectDevice(cfg, "/not-a-device"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfirmDirectDeviceRequiresForceOffline(t *testing.T) {
	devPath := createBlockDevice(t)
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	if err := confirmDirectDevice(cfg, devPath); err == nil || err.Error() != "direct device writes require --force-offline" {
		t.Fatalf("expected force-offline error, got: %v", err)
	}
}

func TestConfirmDirectDeviceTTYRequiresConfirmation(t *testing.T) {
	devPath := createBlockDevice(t)
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.ForceOffline = true

	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty open error: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = slave
	defer func() {
		os.Stdin = origStdin
		master.Close()
		slave.Close()
	}()
	fmt.Fprintln(master, "no")

	if err := confirmDirectDevice(cfg, devPath); err == nil || err.Error() != "direct device write cancelled" {
		t.Fatalf("expected cancellation error, got: %v", err)
	}
}

func TestConfirmDirectDeviceTTYConfirmed(t *testing.T) {
	devPath := createBlockDevice(t)
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.ForceOffline = true

	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty open error: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = slave
	defer func() {
		os.Stdin = origStdin
		master.Close()
		slave.Close()
	}()
	fmt.Fprintln(master, "double-confirm")

	if err := confirmDirectDevice(cfg, devPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfirmDirectDeviceNonTTYRequiresYesIKnow(t *testing.T) {
	devPath := createBlockDevice(t)
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.ForceOffline = true

	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
		r.Close()
		w.Close()
	}()

	if err := confirmDirectDevice(cfg, devPath); err == nil || err.Error() != "direct device writes require --yes-i-know flag when not run interactively" {
		t.Fatalf("expected yes-i-know error, got: %v", err)
	}
}

func TestConfirmDirectDeviceNonTTYConfirmed(t *testing.T) {
	devPath := createBlockDevice(t)
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.ForceOffline = true
	cfg.YesIKnow = true

	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
		r.Close()
		w.Close()
	}()

	if err := confirmDirectDevice(cfg, devPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
