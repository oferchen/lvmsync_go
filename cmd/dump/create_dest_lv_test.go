package dump

import (
	"context"
	"io"
	"os"
	"testing"

	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/device"
	"github.com/oferchen/lvmsync_go/internal/config"
	"github.com/oferchen/lvmsync_go/lvm"
	"github.com/oferchen/lvmsync_go/transfer"
)

func TestRunLocalDumpCreatesDestLV(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	cfg.DedupStrategy = "none"
	cfg.Parallel = 1
	cfg.SpeedLimit = 0
	cfg.CreateDestLV = true

	var createCalled bool
	r := NewRunnerWithDeps(&Runner{
		dumpSeq:        func(context.Context, *transfer.Transfer, *config.Config, string, string, io.Writer) error { return nil },
		openFile:       func(string, int, os.FileMode) (*os.File, error) { return os.OpenFile(os.DevNull, os.O_RDWR, 0) },
		verifyIdentity: func(context.Context, *device.Info, string, string) error { return nil },
		createLV: func(_ context.Context, vg, lv string, size uint64, _ *zap.Logger) error {
			createCalled = true
			if vg != "vg" || lv != "dest" || size != 1234 {
				t.Fatalf("unexpected create params %s %s %d", vg, lv, size)
			}
			return nil
		},
		parseLVPath:   func(_ string) (string, string, error) { return "vg", "dest", nil },
		getVolumeSize: func(_ string, _ *lvm.FDCache, _ *zap.Logger) (uint64, error) { return 1234, nil },
		newFDC:        lvm.NewDeviceFDCache,
	})
	if _, err := r.RunLocalDump(context.Background(), cfg, "snap", "orig", "/dev/vg/dest", zap.NewNop()); err != nil {
		t.Fatalf("RunLocalDump error: %v", err)
	}
	if !createCalled {
		t.Fatalf("createLV not called")
	}
}
