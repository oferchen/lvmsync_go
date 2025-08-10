package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/app"
	applycmd "lvmsync_go/cmd/apply"
	clientcmd "lvmsync_go/cmd/client"
	"lvmsync_go/config"
	clientpkg "lvmsync_go/internal/client"
	"lvmsync_go/internal/privesc"
	"lvmsync_go/lvm"
)

var (
	configureFunc = configure
	runFunc       = run
	exitFunc      = os.Exit
)

// syncLogger flushes any buffered log entries and logs if the sync fails.
func syncLogger(logger *zap.Logger) {
	if err := logger.Sync(); err != nil {
		logger.Error("Logger sync error", zap.Error(err))
	}
}

func configure() (*config.Config, *zap.Logger, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("configuration error: %w", err)
	}

	if err = privesc.EnsureRoot(cfg.LVMEscalation, unix.Exec); err != nil {
		return nil, nil, fmt.Errorf("privilege escalation error: %w", err)
	}

	if err = cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("configuration validation error: %w", err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return nil, nil, fmt.Errorf("logger initialization error: %w", err)
	}

	logger.Info("Effective configuration",
		zap.String("block_size", cfg.HumanBlockSize()),
		zap.Int("parallel", cfg.Parallel),
		zap.String("dedup_strategy", cfg.DedupStrategy),
		zap.String("compress", cfg.Compress),
		zap.Int("compress_level", cfg.CompressLevel),
		zap.Int("compress_concurrency", cfg.CompressConcurrency),
		zap.String("snapshot_size", cfg.SnapshotSize),
		zap.String("volume_group", cfg.VolumeGroup),
		zap.String("target_volume_group", cfg.TargetVolumeGroup),
		zap.Bool("stdout_mode", cfg.StdoutMode),
		zap.String("lvmsync_path", cfg.LVMSyncPath),
		zap.String("grpc_listen", cfg.GRPCListen),
		zap.String("grpc_connect", cfg.GRPCConnect),
	)

	return cfg, logger, nil
}

func run(cfg *config.Config, logger *zap.Logger) error {
	defer lvm.Cleanup()

	if err := clientcmd.SelectTransport(cfg, logger); err != nil {
		return err
	}

	cleanupSrv, err := app.StartGRPCServer(cfg, logger)
	if err != nil {
		return err
	}
	defer cleanupSrv()

	cleanupClient, err := app.ClientHandshake(cfg, logger)
	if err != nil {
		return err
	}
	defer cleanupClient()

	var snapshotPath string
	signals, sigErrCh := app.SetupSignalHandling(cfg, &snapshotPath)
	defer signal.Stop(signals)

	if cfg.ApplyMode != "" {
		if err = applycmd.Run(cfg, cfg.ApplyMode, pflag.Args()); err != nil {
			return fmt.Errorf("apply operation failed: %w", err)
		}
		return nil
	}

	args := pflag.Args()
	if (cfg.StdoutMode && len(args) < 1) || (!cfg.StdoutMode && len(args) < 2) {
		pflag.Usage()
		return fmt.Errorf("invalid arguments")
	}
	originalVolume := args[0]
	destPath := ""
	if !cfg.StdoutMode {
		destPath = args[1]
	}

	var monitorErrCh chan error
	snapshotPath, monitorErrCh, cleanup, err := app.PrepareSnapshot(cfg, originalVolume, logger)
	if err != nil {
		return err
	}
	defer cleanup()

	return clientpkg.ExecuteClient(func(snapshot, dest string) error {
		return clientcmd.Run(cfg, snapshot, dest, logger)
	}, snapshotPath, destPath, sigErrCh, monitorErrCh)
}

func main() {
	cfg, logger, err := configureFunc()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration failed: %v\n", err)
		os.Exit(1)
	}
	if err := runFunc(cfg, logger); err != nil {
		logger.Error("run failed", zap.Error(err))
		syncLogger(logger)
		exitFunc(1)
	}
	syncLogger(logger)
}
