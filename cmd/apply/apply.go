package apply

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	cpufeatures "lvmsync_go/internal/cpufeatures"
	digestpkg "lvmsync_go/internal/digest"
	"lvmsync_go/transfer"
)

// applyFunc allows tests to override the apply implementation.
var (
	applyFunc = func(cfg *config.Config, applyFile, destDevice string, logger *zap.Logger) error {
		t := transfer.NewTransfer(logger, &sync.WaitGroup{})
		return t.RunApply(cfg, applyFile, destDevice)
	}
	detectDevice = device.Detect
)

func init() {
	rootcmd.RunApply = Run
}

// Run executes apply mode using the provided configuration and arguments.
// args should contain the destination device as the first element.
func Run(cfg *config.Config, applyFile string, args []string, logger *zap.Logger) error {
	defer rootcmd.SyncLogger(logger)
	if len(args) < 1 {
		return fmt.Errorf("no destination device specified for apply mode")
	}
	destPath := args[0]
	dev, err := detectDevice(context.Background(), destPath, cfg.Offline, cfg.DestType, cfg.FSFreezeCommand, cfg.FSThawCommand, cfg.LVMEscalation, cfg.FreezeTimeout, cfg.ThawTimeout, logger)
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
	err = applyFunc(cfg, applyFile, dev.Path(), logger)
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
	alg := strings.ToLower(cfg.ChecksumAlgorithm)
	if alg == "" || alg == "auto" {
		alg = digestpkg.Select()
	}
	logger.Info("cpu_features",
		zap.Bool("avx2", cpufeatures.HasAVX2()),
		zap.Bool("avx512", cpufeatures.HasAVX512()),
		zap.Bool("neon", cpufeatures.HasNEON()),
	)
	sampled := strings.ToLower(cfg.VerifyLevel) == "sampled"
	match, srcSum, dstSum, err := digestpkg.VerifyFiles(applyFile, dev.Path(), alg, sampled)
	if err != nil {
		return err
	}
	if !match {
		logger.Error("digest_mismatch",
			zap.String("source_digest", fmt.Sprintf("%x", srcSum[:])),
			zap.String("dest_digest", fmt.Sprintf("%x", dstSum[:])),
		)
		return fmt.Errorf("digest mismatch")
	}
	logger.Info("verification_success",
		zap.String("digest_algorithm", alg),
		zap.String("verify_level", cfg.VerifyLevel),
	)
	return nil
}
