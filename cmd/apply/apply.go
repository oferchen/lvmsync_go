package apply

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	cpufeatures "lvmsync_go/internal/cpufeatures"
	digestpkg "lvmsync_go/internal/digest"
	"lvmsync_go/transfer"
)

// Runner manages external interactions for the apply command.
type Runner struct {
	applyFunc    func(*config.Config, string, string, *zap.Logger) error
	detectDevice func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, *zap.Logger, *device.Runner) (device.Device, error)
}

// NewRunner constructs a Runner with production dependencies.
func NewRunner() *Runner {
	return &Runner{
		applyFunc: func(cfg *config.Config, applyFile, destDevice string, logger *zap.Logger) error {
			t := transfer.NewTransfer(logger, &sync.WaitGroup{}, nil)
			return t.RunApply(cfg, applyFile, destDevice)
		},
		detectDevice: device.Detect,
	}
}

// NewRunnerWithDeps constructs a Runner overriding defaults.
func NewRunnerWithDeps(deps *Runner) *Runner {
	r := NewRunner()
	if deps == nil {
		return r
	}
	if deps.applyFunc != nil {
		r.applyFunc = deps.applyFunc
	}
	if deps.detectDevice != nil {
		r.detectDevice = deps.detectDevice
	}
	return r
}

func init() {
	rootcmd.RegisterApply(NewRunner().Run)
}

// Run executes apply mode using the provided configuration and arguments.
// args should contain the destination device as the first element.
func (r *Runner) Run(cfg *config.Config, applyFile string, args []string, logger *zap.Logger) error {
	defer rootcmd.SyncLogger(logger)
	if len(args) < 1 {
		return fmt.Errorf("no destination device specified for apply mode")
	}
	destPath := args[0]
	dev, err := r.detectDevice(
		context.Background(),
		destPath,
		cfg.Offline,
		cfg.DestType,
		cfg.FSFreezeCommand,
		cfg.FSThawCommand,
		cfg.LVMEscalation,
		cfg.FreezeTimeout,
		cfg.ThawTimeout,
		logger,
		device.NewRunner(),
	)
	if err != nil {
		return err
	}
	switch dev.(type) {
	case *device.RawDevice:
		cfg.DestType = "raw"
	case *device.LVMDevice:
		cfg.DestType = "lvm"
	case *device.FileDevice:
		cfg.DestType = "file"
	}
	if cfg.DestType == "raw" && !cfg.SkipSnapshotCreation {
		dev.Cleanup(context.Background())
		dev.Close()
		return fmt.Errorf("raw destinations require --skip_snapshot_creation or external freeze hooks")
	}
	err = r.applyFunc(cfg, applyFile, dev.Path(), logger)
	cleanupErr := dev.Cleanup(context.Background())
	closeErr := dev.Close()
	if err != nil {
		return err
	}
	if cleanupErr != nil {
		return cleanupErr
	}
	if closeErr != nil {
		return closeErr
	}
	if strings.ToLower(cfg.VerifyLevel) == "none" {
		return nil
	}
	expectedHex := os.Getenv("LVMSYNC_SOURCE_DIGEST")
	if expectedHex == "" {
		return fmt.Errorf("missing source digest")
	}
	expected, err := hex.DecodeString(expectedHex)
	if err != nil {
		return fmt.Errorf("decode source digest: %w", err)
	}
	if len(expected) != 32 {
		return fmt.Errorf("invalid source digest length")
	}
	var srcSum [32]byte
	copy(srcSum[:], expected)
	alg := strings.ToLower(cfg.ChecksumAlgorithm)
	if alg == "" || alg == "auto" {
		alg = digestpkg.Select()
	}
	logger.Info("cpu_features",
		zap.Bool("avx2", cpufeatures.HasAVX2()),
		zap.Bool("avx512", cpufeatures.HasAVX512()),
		zap.Bool("neon", cpufeatures.HasNEON()),
	)
	var dstSum [32]byte
	sampled := strings.ToLower(cfg.VerifyLevel) == "sampled"
	if sampled {
		dstSum, err = digestpkg.SampledSumFile(dev.Path(), alg)
	} else {
		dstSum, err = digestpkg.SumFile(dev.Path(), alg)
	}
	if err != nil {
		return err
	}
	if srcSum != dstSum {
		logger.Error("digest_mismatch",
			zap.String("digest_alg", alg),
			zap.String("source_digest", fmt.Sprintf("%x", srcSum[:])),
			zap.String("dest_digest", fmt.Sprintf("%x", dstSum[:])),
		)
		return fmt.Errorf("digest mismatch")
	}
	logger.Info("verification_success",
		zap.String("digest_alg", alg),
		zap.String("verify_level", cfg.VerifyLevel),
	)
	return nil
}
