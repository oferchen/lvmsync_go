package root

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"golang.org/x/term"

	"lvmsync_go/app"
	clientpkg "lvmsync_go/internal/client"
	"lvmsync_go/internal/config"
	"lvmsync_go/internal/exitcode"
	"lvmsync_go/internal/logging"
	"lvmsync_go/internal/privilege"
	"lvmsync_go/lvm"
	"lvmsync_go/transport"
)

// Runner manages external interactions for the root command.
type Runner struct {
	setupSignalHandleFn func(context.Context, *config.Config, *string, *zap.Logger) (chan os.Signal, chan error)
	prepareSnapshotFn   func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error)
	executeClientFn     func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error
	selectTransportFn   func(cfg *config.Config, logger *zap.Logger) (transport.Interface, error)
	runDumpFn           func(ctx context.Context, cfg *config.Config, snapshot, dest string, logger *zap.Logger) (string, error)
	runManifestFn       func(cfg *config.Config, args []string, logger *zap.Logger) error
	runVerifyFn         func(args []string, logger *zap.Logger) error
}

// NewRunner constructs a Runner with production dependencies.
func NewRunner() *Runner {
	return &Runner{
		setupSignalHandleFn: app.SetupSignalHandling,
		prepareSnapshotFn:   app.PrepareSnapshot,
		executeClientFn:     clientpkg.ExecuteClient,
		selectTransportFn: func(cfg *config.Config, logger *zap.Logger) (transport.Interface, error) {
			return nil, fmt.Errorf("transport not registered")
		},
		runDumpFn: func(ctx context.Context, cfg *config.Config, snapshot, dest string, logger *zap.Logger) (string, error) {
			return "", fmt.Errorf("dump command not registered")
		},
		runManifestFn: func(cfg *config.Config, args []string, logger *zap.Logger) error {
			return fmt.Errorf("manifest command not registered")
		},
		runVerifyFn: func(args []string, logger *zap.Logger) error {
			return fmt.Errorf("verify command not registered")
		},
	}
}

// NewRunnerWithDeps constructs a Runner overriding default dependencies.
func NewRunnerWithDeps(deps *Runner) *Runner {
	r := NewRunner()
	if deps == nil {
		return r
	}
	if deps.setupSignalHandleFn != nil {
		r.setupSignalHandleFn = deps.setupSignalHandleFn
	}
	if deps.prepareSnapshotFn != nil {
		r.prepareSnapshotFn = deps.prepareSnapshotFn
	}
	if deps.executeClientFn != nil {
		r.executeClientFn = deps.executeClientFn
	}
	if deps.selectTransportFn != nil {
		r.selectTransportFn = deps.selectTransportFn
	}
	if deps.runDumpFn != nil {
		r.runDumpFn = deps.runDumpFn
	}
	if deps.runManifestFn != nil {
		r.runManifestFn = deps.runManifestFn
	}
	if deps.runVerifyFn != nil {
		r.runVerifyFn = deps.runVerifyFn
	}
	return r
}

var defaultRunner = NewRunner()

// RegisterDump sets the dump handler used by the default Runner.
func RegisterDump(fn func(context.Context, *config.Config, string, string, *zap.Logger) (string, error)) {
	defaultRunner.runDumpFn = fn
}

// RegisterSelectTransport sets the transport selector used by the default Runner.
func RegisterSelectTransport(fn func(*config.Config, *zap.Logger) (transport.Interface, error)) {
	defaultRunner.selectTransportFn = fn
}

// RegisterManifest sets the manifest handler used by the default Runner.
func RegisterManifest(fn func(*config.Config, []string, *zap.Logger) error) {
	defaultRunner.runManifestFn = fn
}

// RegisterVerify sets the verify handler used by the default Runner.
func RegisterVerify(fn func([]string, *zap.Logger) error) {
	defaultRunner.runVerifyFn = fn
}

// SyncLogger flushes buffered log entries and logs if syncing fails.
func SyncLogger(logger *zap.Logger) {
       if err := logger.Sync(); err != nil {
               logger.Error("logger_sync_error", zap.Error(err))
       }
}

// ConfigureWithEscalator loads configuration, ensures privileges via the provided
// privilege.Escalator, validates, and sets up logging. A nil escalator defaults
// to the production implementation.
func ConfigureWithEscalator(esc privilege.Escalator) (*config.Config, []string, *zap.Logger, error) {
	defaults, err := config.DefaultConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("configuration error: %w", err)
	}
	builder := config.NewBuilder(defaults)
	fs := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	cfg, args, warns, err := builder.Build(fs, os.Args[1:])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("configuration error: %w", err)
	}
	if cfg.StdoutMode && !cfg.YesIKnow {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprint(os.Stderr, "This will write binary data to your terminal. Continue? [y/N]: ")
			resp, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			resp = strings.TrimSpace(strings.ToLower(resp))
			if resp != "y" && resp != "yes" {
				return nil, nil, nil, fmt.Errorf("stdout mode requires confirmation")
			}
		} else {
			return nil, nil, nil, fmt.Errorf("stdout mode requires --yes-i-know flag when not run interactively")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.LVMTimeout)
	defer cancel()
	if esc == nil {
		esc = privilege.New(ctx)
	}
	if err = esc.Ensure(ctx); err != nil {
		return nil, nil, nil, fmt.Errorf("privilege check failed: %w", err)
	}
	if err = cfg.Validate(); err != nil {
		return nil, nil, nil, fmt.Errorf("configuration validation error: %w", err)
	}
	logger, err := logging.NewLogger(cfg, "root")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("logger initialization error: %w", err)
	}
	for _, w := range warns {
		logger.Warn(w)
	}
	logger.Info("Effective configuration",
		zap.String("block_size", cfg.HumanBlockSize()),
		zap.Uint64("block_size_bytes", cfg.BlockSizeBytes()),
		zap.Int("parallel", cfg.Parallel),
		zap.String("transport", cfg.Transport),
		zap.Int("concurrency", cfg.Concurrency),
		zap.Int("tcp_port", cfg.TCPPort),
		zap.String("dedup_strategy", cfg.DedupStrategy),
		zap.String("compress", cfg.Compress),
		zap.Int("compress_level", cfg.CompressLevel),
		zap.Int("compress_concurrency", cfg.CompressConcurrency),
		zap.String("snapshot_size", cfg.SnapshotSize),
		zap.String("volume_group", cfg.VolumeGroup),
		zap.String("target_volume_group", cfg.TargetVolumeGroup),
		zap.Bool("stdout_mode", cfg.StdoutMode),
		zap.String("lvmsync_path", cfg.LVMSyncPath),
	)
	return cfg, args, logger, nil
}

// Configure loads configuration, ensures privileges, validates, and sets up
// logging using the default privilege escalator.
func Configure() (*config.Config, []string, *zap.Logger, error) {
	return ConfigureWithEscalator(nil)
}

// PrepareSnapshot wraps snapshot preparation.
func (r *Runner) PrepareSnapshot(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return r.prepareSnapshotFn(ctx, cfg, originalVolume, logger)
}

// ExecuteClient runs the client transfer logic.
func (r *Runner) ExecuteClient(ctx context.Context, cfg *config.Config, snapshotPath, destPath string, sigErrCh, monitorErrCh chan error, logger *zap.Logger) error {
	return r.executeClientFn(ctx, func(ctx context.Context, snapshot, dest string) error {
		_, err := r.runDumpFn(ctx, cfg, snapshot, dest, logger)
		return err
	}, snapshotPath, destPath, sigErrCh, monitorErrCh, logger)
}

// PrepareSnapshot wraps the default runner's PrepareSnapshot.
func PrepareSnapshot(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return defaultRunner.PrepareSnapshot(ctx, cfg, originalVolume, logger)
}

// ExecuteClient wraps the default runner's ExecuteClient.
func ExecuteClient(ctx context.Context, cfg *config.Config, snapshotPath, destPath string, sigErrCh, monitorErrCh chan error, logger *zap.Logger) error {
	return defaultRunner.ExecuteClient(ctx, cfg, snapshotPath, destPath, sigErrCh, monitorErrCh, logger)
}

// Run orchestrates the command execution.
func (r *Runner) dispatchSubcommand(cfg *config.Config, args []string, logger *zap.Logger) (bool, error) {
	if len(args) > 0 {
		switch args[0] {
		case "manifest":
			return true, r.runManifestFn(cfg, args[1:], logger)
		case "verify":
			return true, r.runVerifyFn(args[1:], logger)
		}
	}
	return false, nil
}

func (r *Runner) prepareClient(cfg *config.Config, args []string, logger *zap.Logger) (context.Context, func(), string, string, chan error, chan error, error) {
	if _, err := r.selectTransportFn(cfg, logger); err != nil {
		return nil, nil, "", "", nil, nil, fmt.Errorf("select transport: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var snapshotPath string
	signals, sigErrCh := r.setupSignalHandleFn(ctx, cfg, &snapshotPath, logger)

	if (cfg.StdoutMode && len(args) < 1) || (!cfg.StdoutMode && len(args) < 2) {
		pflag.Usage()
		cancel()
		signal.Stop(signals)
		return nil, nil, "", "", nil, nil, fmt.Errorf("invalid arguments")
	}

	originalVolume := args[0]
	destPath := ""
	if !cfg.StdoutMode {
		destPath = args[1]
	}

	snapPath, monitorErrCh, snapCleanup, err := r.prepareSnapshotFn(ctx, cfg, originalVolume, logger)
	if err != nil {
		cancel()
		signal.Stop(signals)
		if snapCleanup != nil {
			snapCleanup()
		}
		return nil, nil, "", "", nil, nil, fmt.Errorf("prepare snapshot: %w", err)
	}
	snapshotPath = snapPath

	cleanup := func() {
		if snapCleanup != nil {
			snapCleanup()
		}
		signal.Stop(signals)
		cancel()
	}
	return ctx, cleanup, snapshotPath, destPath, sigErrCh, monitorErrCh, nil
}

func (r *Runner) executeSync(ctx context.Context, cfg *config.Config, snapshotPath, destPath string, sigErrCh, monitorErrCh chan error, logger *zap.Logger) error {
	return r.ExecuteClient(ctx, cfg, snapshotPath, destPath, sigErrCh, monitorErrCh, logger)
}

func (r *Runner) Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	defer lvm.CleanupRegistered(context.Background())
	handled, err := r.dispatchSubcommand(cfg, args, logger)
	if handled || err != nil {
		return err
	}
	if cfg.Plan {
		return emitPlan(cfg, args, logger)
	}

	ctx, cleanup, snapshotPath, destPath, sigErrCh, monitorErrCh, err := r.prepareClient(cfg, args, logger)
	if err != nil {
		return err
	}
	defer cleanup()

	return r.executeSync(ctx, cfg, snapshotPath, destPath, sigErrCh, monitorErrCh, logger)
}

// Run invokes the default runner's Run method.
func Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	return defaultRunner.Run(cfg, args, logger)
}

// Execute is a helper that configures and runs the root command.
func Execute() error {
	cfg, args, logger, err := Configure()
	if err != nil {
		return err
	}
	if err := Run(cfg, args, logger); err != nil {
		logger.Error("run failed", zap.Error(err))
		SyncLogger(logger)
		return err
	}
	SyncLogger(logger)
	return nil
}

// ExitCode maps an error to an exit code.
func ExitCode(err error) int {
	if err == nil {
		return exitcode.OK
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "privilege check"):
		return exitcode.ErrCapability
	case strings.Contains(msg, "parse") || strings.Contains(msg, "missing") || strings.Contains(msg, "required") || strings.Contains(msg, "invalid") || strings.Contains(msg, "config"):
		return exitcode.ErrConfig
	case strings.Contains(msg, "precondition"):
		return exitcode.ErrPrecondition
	case strings.Contains(msg, "resumable") || strings.Contains(msg, "resume"):
		return exitcode.ErrResumable
	case strings.Contains(msg, "snapshot exhausted"):
		return exitcode.ErrSnapshotExhausted
	case strings.Contains(msg, "device"):
		return exitcode.ErrDevice
	case strings.Contains(msg, "mismatch") || strings.Contains(msg, "blocks differ"):
		return exitcode.ErrVerify
	case strings.Contains(msg, "signal") || errors.Is(err, context.Canceled):
		return exitcode.ErrPartial
	default:
		return exitcode.ErrRuntime
	}
}
