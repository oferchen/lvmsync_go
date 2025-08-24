package root

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/creack/pty"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/internal/config"
	"lvmsync_go/transport"
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

func newStubRunner(called *bool) *Runner {
	return NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		setupSignalHandleFn: func(context.Context, *config.Config, *string, *zap.Logger) (chan os.Signal, chan error) {
			return make(chan os.Signal), make(chan error)
		},
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			*called = true
			return "snap", make(chan error), func() {}, nil
		},
	})
}

func TestPrepareClientDirectDeviceTTYRequiresConfirmation(t *testing.T) {
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

	called := false
	rnr := newStubRunner(&called)
	if _, _, _, _, _, _, err := rnr.prepareClient(cfg, []string{"/src", devPath}, zap.NewNop()); err == nil || err.Error() != "direct device write cancelled" {
		t.Fatalf("expected cancellation error, got: %v", err)
	}
	if called {
		t.Fatalf("prepareSnapshot should not be called")
	}
}

func TestPrepareClientDirectDeviceTTYConfirmed(t *testing.T) {
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

	called := false
	rnr := newStubRunner(&called)
	if _, _, _, _, _, _, err := rnr.prepareClient(cfg, []string{"/src", devPath}, zap.NewNop()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("prepareSnapshot not called")
	}
}

func TestPrepareClientDirectDeviceNonTTYRequiresYesIKnow(t *testing.T) {
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

	called := false
	rnr := newStubRunner(&called)
	if _, _, _, _, _, _, err := rnr.prepareClient(cfg, []string{"/src", devPath}, zap.NewNop()); err == nil || err.Error() != "direct device writes require --yes-i-know flag when not run interactively" {
		t.Fatalf("expected yes-i-know error, got: %v", err)
	}
	if called {
		t.Fatalf("prepareSnapshot should not be called")
	}
}

func TestPrepareClientDirectDeviceNonTTYConfirmed(t *testing.T) {
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

	called := false
	rnr := newStubRunner(&called)
	if _, _, _, _, _, _, err := rnr.prepareClient(cfg, []string{"/src", devPath}, zap.NewNop()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("prepareSnapshot not called")
	}
}
