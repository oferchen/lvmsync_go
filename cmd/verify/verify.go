package verify

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/zeebo/blake3"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	rootcmd "github.com/oferchen/lvmsync_go/cmd/root"
	"github.com/oferchen/lvmsync_go/device"
	"github.com/oferchen/lvmsync_go/internal/blockio"
	"github.com/oferchen/lvmsync_go/internal/config"
	cpufeatures "github.com/oferchen/lvmsync_go/internal/cpufeatures"
	digestpkg "github.com/oferchen/lvmsync_go/internal/digest"
	privilege "github.com/oferchen/lvmsync_go/internal/privilege"
	manifestpkg "github.com/oferchen/lvmsync_go/manifest"
)

// Runner holds dependencies for verify operations.
type Runner struct {
	Rebuild func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error
	Detect  func(ctx context.Context, path string, snap, offline bool, typ, fsFreeze, fsThaw, lvmEsc string, freezeTimeout, thawTimeout time.Duration, esc privilege.Escalator, logger *zap.Logger, runner *device.Runner) (device.Device, error)
	verify  func(ctx context.Context, cfg *config.Config, src, dst, manifestPath string, logger *zap.Logger) error
}

// NewRunner returns a Runner with production dependencies.
func NewRunner() *Runner {
	r := &Runner{Rebuild: manifestpkg.Rebuild, Detect: device.Detect}
	r.verify = r.verifyDevices
	return r
}

// NewRunnerWithDeps creates a Runner with custom dependencies.
func NewRunnerWithDeps(
	rebuild func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error,
	detect func(ctx context.Context, path string, snap, offline bool, typ, fsFreeze, fsThaw, lvmEsc string, freezeTimeout, thawTimeout time.Duration, esc privilege.Escalator, logger *zap.Logger, runner *device.Runner) (device.Device, error),
	verify func(ctx context.Context, cfg *config.Config, src, dst, manifestPath string, logger *zap.Logger) error,
) *Runner {
	r := &Runner{Rebuild: rebuild}
	if detect != nil {
		r.Detect = detect
	} else {
		r.Detect = device.Detect
	}
	if verify != nil {
		r.verify = verify
	} else {
		r.verify = r.verifyDevices
	}
	return r
}

func init() {
	r := NewRunner()
	rootcmd.RegisterVerify(r.Run)
}

// Run executes the verify command with the provided arguments and logger.
// Args should exclude the "verify" subcommand itself.
func (r *Runner) Run(args []string, logger *zap.Logger) error {
	defer rootcmd.SyncLogger(logger)
	cmd := &cobra.Command{
		Use:                "verify [flags] <source> <dest>",
		Short:              "Verify that source and destination contain identical data",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, argv []string) error {
			ctx := cmd.Context()
			defaults, err := config.DefaultConfig()
			if err != nil {
				return err
			}
			builder := config.NewBuilder(defaults)
			builder.FlagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
			builder.FlagSets.Remote = pflag.NewFlagSet("Remote Options", pflag.ExitOnError)
			builder.FlagSets.Compression = pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
			builder.FlagSets.LVM = pflag.NewFlagSet("LVM Options", pflag.ExitOnError)
			builder.FlagSets.Transport = pflag.NewFlagSet("Transport Options", pflag.ExitOnError)
			fs := pflag.NewFlagSet("verify", pflag.ContinueOnError)
			cfg, remaining, warns, err := builder.Build(fs, argv)
			if err != nil {
				return err
			}
			for _, w := range warns {
				fields := []zap.Field{zap.String("message", w)}
				switch {
				case strings.HasPrefix(w, "unknown configuration key "):
					key := strings.Trim(strings.TrimPrefix(w, "unknown configuration key "), "\"")
					fields = append(fields,
						zap.String("config_key", key),
						zap.String("reason", "unknown_config_key"),
					)
				case strings.Contains(w, "allow_insecure enabled"):
					fields = append(fields, zap.String("reason", "allow_insecure_enabled"))
				}
				logger.Warn("configuration_warning", fields...)
			}
			if len(remaining) != 2 {
				fs.Usage()
				return fmt.Errorf("usage: lvmsync verify [flags] <source> <dest>")
			}
			if cfg.DryRun {
				info, err := os.Stat(remaining[0])
				if err != nil {
					return fmt.Errorf("stat source: %w", err)
				}
				size := info.Size()
				var eta time.Duration
				if cfg.SpeedLimit > 0 {
					eta = time.Duration(size/int64(cfg.SpeedLimit)) * time.Second
				}
				logger.Info(
					"dry run",
					zap.Int64("size_bytes", size),
					zap.Int64("eta_ms", eta.Milliseconds()),
				)
				return nil
			}
			err = r.verify(ctx, cfg, remaining[0], remaining[1], cfg.ManifestPath, logger)
			if cfg.Output == "json" || cfg.Output == "yaml" {
				out := struct {
					Verified bool   `json:"verified" yaml:"verified"`
					Error    string `json:"error,omitempty" yaml:"error,omitempty"`
				}{Verified: err == nil}
				if err != nil {
					out.Error = err.Error()
				}
				switch cfg.Output {
				case "json":
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					if encErr := enc.Encode(out); encErr != nil {
						return fmt.Errorf("encode json: %w", encErr)
					}
				case "yaml":
					enc := yaml.NewEncoder(os.Stdout)
					enc.SetIndent(2)
					if encErr := enc.Encode(out); encErr != nil {
						return fmt.Errorf("encode yaml: %w", encErr)
					}
					if closeErr := enc.Close(); closeErr != nil {
						return fmt.Errorf("close yaml encoder: %w", closeErr)
					}
				}
			}
			return err
		},
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

func (r *Runner) verifyDevices(ctx context.Context, cfg *config.Config, src, dst, manifestPath string, logger *zap.Logger) error {
	esc, err := privilege.New(ctx, logger)
	if err != nil {
		return err
	}
	runner := device.NewRunner()

	srcDev, err := r.Detect(ctx, src, true, cfg.Offline, cfg.SourceType, cfg.FSFreezeCommand, cfg.FSThawCommand, cfg.LVMEscalation, cfg.FreezeTimeout, cfg.ThawTimeout, esc, logger, runner)
	if err != nil {
		return err
	}
	srcSnap, err := srcDev.Snapshot(ctx, cfg.SnapshotSize)
	if err != nil {
		srcDev.Close()
		return err
	}
	srcPath := srcSnap.Path()
	defer func() {
		_ = srcSnap.Cleanup(ctx)
		_ = srcSnap.Close()
		if srcSnap != srcDev {
			_ = srcDev.Close()
		}
	}()

	dstDev, err := r.Detect(ctx, dst, true, cfg.Offline, cfg.DestType, cfg.FSFreezeCommand, cfg.FSThawCommand, cfg.LVMEscalation, cfg.FreezeTimeout, cfg.ThawTimeout, esc, logger, runner)
	if err != nil {
		return err
	}
	dstSnap, err := dstDev.Snapshot(ctx, cfg.SnapshotSize)
	if err != nil {
		dstDev.Close()
		return err
	}
	dstPath := dstSnap.Path()
	defer func() {
		_ = dstSnap.Cleanup(ctx)
		_ = dstSnap.Close()
		if dstSnap != dstDev {
			_ = dstDev.Close()
		}
	}()

	if manifestPath == "" {
		manifestPath = srcPath + ".manifest"
	}
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			mctx := ctx
			if cfg.ManifestTimeout > 0 {
				var cancel context.CancelFunc
				mctx, cancel = context.WithTimeout(mctx, cfg.ManifestTimeout)
				defer cancel()
			}
			hybridFixed := uint32(0)
			if cfg.DedupMode == "hybrid" {
				hybridFixed = uint32(cfg.BlockSize)
			}
			if err := r.Rebuild(mctx, srcPath, manifestPath, logger, cfg.ManifestProgressInterval, cfg.ManifestAllowMounted, uint32(cfg.CDCMin), uint32(cfg.CDCAvg), uint32(cfg.CDCMax), hybridFixed); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				return fmt.Errorf("rebuild manifest: %w", err)
			}
		} else {
			return fmt.Errorf("stat manifest: %w", err)
		}
	}
	if err := verifyWithManifest(cfg, dstPath, manifestPath, logger); err != nil {
		return err
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
	post := strings.ToLower(cfg.VerifyLevel) == "post"
	match, srcSum, dstSum, err := digestpkg.VerifyFiles(srcPath, dstPath, alg, post)
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

// Run executes using a default Runner.
func Run(args []string, logger *zap.Logger) error {
	return NewRunner().Run(args, logger)
}

// digestFunc returns a 32-byte digest function for the configured algorithm.
// Supported algorithms are "blake3" and "sha256".
// An error is returned for any other value.
func digestFunc(cfg *config.Config) (func([]byte) [32]byte, error) {
	switch strings.ToLower(cfg.ChecksumAlgorithm) {
	case "blake3", "":
		return blake3.Sum256, nil
	case "sha256":
		return sha256.Sum256, nil
	default:
		return nil, fmt.Errorf("unsupported checksum algorithm %q: must be blake3 or sha256", cfg.ChecksumAlgorithm)
	}
}

type blockReader interface {
	Close() error
	Logical() int
	Direct() bool
	ReadAt([]byte, int64) (int, error)
}

func verifyWithManifest(cfg *config.Config, devicePath, manifestPath string, logger *zap.Logger) error {
	hash, err := digestFunc(cfg)
	if err != nil {
		return err
	}
	return verifyWithManifestOpen(func(path string, direct, strict bool) (blockReader, error) {
		return blockio.Open(path, direct, strict)
	}, hash, cfg, devicePath, manifestPath, logger)
}

func verifyWithManifestOpen(open func(string, bool, bool) (blockReader, error), hash func([]byte) [32]byte, cfg *config.Config, devicePath, manifestPath string, logger *zap.Logger) error {
	idx, err := manifestpkg.Open(manifestPath)
	if err != nil {
		return fmt.Errorf("open manifest: %w", err)
	}
	defer idx.Close()
	fSrc, err := open(devicePath, cfg.ODirect, false)
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}

	workers := cfg.Parallel
	if workers < 1 {
		workers = 1
	}
	type job struct {
		off    uint64
		length uint32
		digest [32]byte
	}
	logical := fSrc.Logical()
	jobs := make([]job, 0, idx.ChunkCount())
	unaligned := false
	for i := uint64(0); i < idx.ChunkCount(); i++ {
		off, length, _, _, digest, err := idx.Entry(i)
		if err != nil {
			return fmt.Errorf("manifest entry: %w", err)
		}
		if length == 0 {
			continue
		}
		if off%uint64(logical) != 0 || int(length)%logical != 0 {
			unaligned = true
		}
		jobs = append(jobs, job{off: off, length: length, digest: digest})
	}
	if unaligned && fSrc.Direct() {
		if err := fSrc.Close(); err != nil {
			return fmt.Errorf("close device: %w", err)
		}
		fSrc, err = open(devicePath, false, false)
		if err != nil {
			return fmt.Errorf("open device: %w", err)
		}
	}
	defer fSrc.Close()
	tasks := make(chan job, workers)
	errCh := make(chan error, 1)
	var mismatches int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 0)
			for j := range tasks {
				if int(j.length) > cap(buf) {
					buf = make([]byte, int(j.length))
				}
				b := buf[:int(j.length)]
				if _, err := fSrc.ReadAt(b, int64(j.off)); err != nil {
					select {
					case errCh <- fmt.Errorf("read source: %w", err):
					default:
					}
					return
				}
				actual := hash(b)
				if actual != j.digest {
					atomic.AddInt64(&mismatches, 1)
					logger.Error("digest_mismatch",
						zap.Uint64("offset_bytes", j.off),
						zap.String("expected_digest", fmt.Sprintf("%x", j.digest[:])),
						zap.String("actual_digest", fmt.Sprintf("%x", actual[:])))
				}
			}
		}()
	}
	for _, j := range jobs {
		select {
		case tasks <- j:
		case err := <-errCh:
			close(tasks)
			wg.Wait()
			return err
		}
	}
	close(tasks)
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
	}
	if mismatches > 0 {
		return fmt.Errorf("%d blocks differ", mismatches)
	}
	logger.Info("verification complete")
	return nil
}
