package dump

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	"lvmsync_go/transfer"
)

func TestRunLocalDumpSuccess(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.DedupStrategy = "none"
	cfg.Parallel = 1
	cfg.SpeedLimit = 0

	var openCalled bool
	var dumpCalled bool
	r := NewRunnerWithDeps(&Runner{
		openFile: func(name string, flag int, perm os.FileMode) (*os.File, error) {
			openCalled = true
			if name != "/fake/dest" {
				t.Fatalf("expected dest /fake/dest, got %s", name)
			}
			return os.OpenFile(os.DevNull, os.O_RDWR, 0)
		},
		dumpSeq: func(_ context.Context, _ *transfer.Transfer, c *config.Config, snap, origin string, out io.Writer) error {
			dumpCalled = true
			if snap != "snap" || origin != "orig" {
				t.Fatalf("unexpected devices: %s %s", snap, origin)
			}
			return nil
		},
		verifyIdentity: func(context.Context, *device.Info, string, string) error { return nil },
	})
	originalDestType := cfg.DestType
	destType, err := r.RunLocalDump(context.Background(), cfg, "snap", "orig", "/fake/dest", zap.NewNop())
	if err != nil {
		t.Fatalf("runLocalDump returned error: %v", err)
	}
	if destType != originalDestType {
		t.Fatalf("expected dest type %q, got %q", originalDestType, destType)
	}
	if cfg.DestType != originalDestType {
		t.Fatalf("cfg.DestType was modified: expected %q, got %q", originalDestType, cfg.DestType)
	}
	if !openCalled {
		t.Fatalf("openFile was not called")
	}
	if !dumpCalled {
		t.Fatalf("dumpChangesSequential was not called")
	}
}

func TestRunLocalDumpOpenError(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}

	var openCalled bool
	r := NewRunnerWithDeps(&Runner{
		openFile: func(string, int, os.FileMode) (*os.File, error) {
			openCalled = true
			return nil, errors.New("open error")
		},
		dumpSeq: func(_ context.Context, _ *transfer.Transfer, c *config.Config, snap, origin string, out io.Writer) error {
			t.Fatalf("dumpChangesSequential should not be called on open error")
			return nil
		},
		verifyIdentity: func(context.Context, *device.Info, string, string) error { return nil },
	})
	originalDestType := cfg.DestType
	if destType, err := r.RunLocalDump(context.Background(), cfg, "snap", "orig", "/fake/dest", zap.NewNop()); err == nil {
		t.Fatalf("expected error, got nil")
	} else if destType != originalDestType {
		t.Fatalf("expected dest type %q, got %q", originalDestType, destType)
	}
	if cfg.DestType != originalDestType {
		t.Fatalf("cfg.DestType was modified: expected %q, got %q", originalDestType, cfg.DestType)
	}
	if !openCalled {
		t.Fatalf("openFile was not called")
	}
}

func TestRunLocalDumpPartitionMismatch(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.DedupStrategy = "none"
	cfg.Parallel = 1
	cfg.CheckPartition = true
	cfg.DestType = "file"

	r := NewRunnerWithDeps(&Runner{verifyIdentity: func(context.Context, *device.Info, string, string) error {
		return device.ErrPartitionMismatch
	}})

	if _, err := r.RunLocalDump(context.Background(), cfg, "snap", "orig", "/fake/dest", zap.NewNop()); err == nil || !errors.Is(err, device.ErrPartitionMismatch) {
		t.Fatalf("expected partition mismatch error, got %v", err)
	}
}
