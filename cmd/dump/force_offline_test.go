package dump

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/internal/config"
)

func setupLoopDev(t *testing.T, size int64) (string, func()) {
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

func TestRunRefusesDeviceWithoutForceOffline(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoopDev(t, 1<<20)
	defer cleanup()
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.YesIKnow = true
	deps := &rootcmd.Runner{}
	setField := func(name string, fn any) {
		v := reflect.ValueOf(deps).Elem().FieldByName(name)
		reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem().Set(reflect.ValueOf(fn))
	}
	setField("setupSignalHandleFn", func(ctx context.Context, cfg *config.Config, snapshotPath *string, logger *zap.Logger) (chan os.Signal, chan error) {
		return make(chan os.Signal), make(chan error)
	})
	setField("prepareSnapshotFn", func(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
		return "snap", nil, func() {}, nil
	})
	setField("executeClientFn", func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error {
		return nil
	})
	r := rootcmd.NewRunnerWithDeps(deps)
	if err := r.Run(cfg, []string{loop, loop}, zap.NewNop()); err == nil || !strings.Contains(err.Error(), "--force-offline") {
		t.Fatalf("expected --force-offline error, got %v", err)
	}
}

func TestRunAllowsDeviceWithForceOffline(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoopDev(t, 1<<20)
	defer cleanup()
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.YesIKnow = true
	cfg.ForceOffline = true
	deps := &rootcmd.Runner{}
	setField := func(name string, fn any) {
		v := reflect.ValueOf(deps).Elem().FieldByName(name)
		reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem().Set(reflect.ValueOf(fn))
	}
	setField("setupSignalHandleFn", func(ctx context.Context, cfg *config.Config, snapshotPath *string, logger *zap.Logger) (chan os.Signal, chan error) {
		return make(chan os.Signal), make(chan error)
	})
	setField("prepareSnapshotFn", func(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
		return "snap", nil, func() {}, nil
	})
	setField("executeClientFn", func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error {
		return nil
	})
	r := rootcmd.NewRunnerWithDeps(deps)
	if err := r.Run(cfg, []string{loop, loop}, zap.NewNop()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}
