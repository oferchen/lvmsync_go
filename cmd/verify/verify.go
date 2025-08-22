package verify

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/zeebo/blake3"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/internal/blockio"
	"lvmsync_go/internal/config"
	cpufeatures "lvmsync_go/internal/cpufeatures"
	digestpkg "lvmsync_go/internal/digest"
	manifestpkg "lvmsync_go/manifest"
)

// Runner holds dependencies for verify operations.
type Runner struct {
	Rebuild func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error
}

// NewRunner returns a Runner with production dependencies.
func NewRunner() *Runner { return &Runner{Rebuild: manifestpkg.Rebuild} }

// NewRunnerWithDeps creates a Runner with custom rebuild function.
func NewRunnerWithDeps(
	rebuild func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error,
) *Runner {
	return &Runner{Rebuild: rebuild}
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
				logger.Info("dry run", zap.Int64("size_bytes", size), zap.Duration("eta", eta))
				return nil
			}
			err = r.verifyDevices(cfg, remaining[0], remaining[1], cfg.ManifestPath, logger)
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

func (r *Runner) verifyDevices(cfg *config.Config, src, dst, manifestPath string, logger *zap.Logger) error {
	ctx := context.Background()
	esc := privilege.New(ctx, logger)
	runner := device.NewRunner()

	srcDev, err := device.Detect(ctx, src, cfg.Offline, cfg.SourceType, cfg.FSFreezeCommand, cfg.FSThawCommand, cfg.LVMEscalation, cfg.FreezeTimeout, cfg.ThawTimeout, esc, logger, runner)
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

	dstDev, err := device.Detect(ctx, dst, cfg.Offline, cfg.DestType, cfg.FSFreezeCommand, cfg.FSThawCommand, cfg.LVMEscalation, cfg.FreezeTimeout, cfg.ThawTimeout, esc, logger, runner)
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
			mctx := context.Background()
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

func verifyInline(cfg *config.Config, src, dst string, logger *zap.Logger) error {
	blockSize := cfg.BlockSize
	if blockSize == 0 {
		blockSize = 8 * 1024 * 1024
	}
	fSrc, err := blockio.Open(src, cfg.ODirect, false)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer fSrc.Close()
	fDst, err := blockio.Open(dst, cfg.ODirect, false)
	if err != nil {
		return fmt.Errorf("open dest: %w", err)
	}
	defer fDst.Close()

	sizeSrc := fSrc.Size()
	sizeDst := fDst.Size()
	if sizeSrc != sizeDst {
		logger.Error("size mismatch", zap.Int64("source_bytes", sizeSrc), zap.Int64("dest_bytes", sizeDst))
		return fmt.Errorf("size mismatch")
	}

	total := sizeSrc
	mismatches := 0
	bufSrc := make([]byte, blockSize)
	bufDst := make([]byte, blockSize)
	digest, err := digestFunc(cfg)
	if err != nil {
		return err
	}
	for off := int64(0); off < total; off += int64(blockSize) {
		size := blockSize
		if remaining := int(total - off); remaining < size {
			size = remaining
		}
		n, err := fSrc.ReadAt(bufSrc[:size], off)
		if err != nil && err != io.EOF {
			return fmt.Errorf("read source: %w", err)
		}
		if n != size {
			return fmt.Errorf("read source: short read: expected %d, got %d", size, n)
		}
		n, err = fDst.ReadAt(bufDst[:size], off)
		if err != nil && err != io.EOF {
			return fmt.Errorf("read dest: %w", err)
		}
		if n != size {
			return fmt.Errorf("read dest: short read: expected %d, got %d", size, n)
		}
		if digest(bufSrc[:size]) != digest(bufDst[:size]) {
			mismatches++
			logger.Error("mismatched_block", zap.Int64("offset_bytes", off))
		}
	}
	if mismatches > 0 {
		return fmt.Errorf("%d blocks differ", mismatches)
	}
	logger.Info("verification complete")
	return nil
}

func verifyWithManifest(cfg *config.Config, devicePath, manifestPath string, logger *zap.Logger) error {
	idx, err := manifestpkg.Open(manifestPath)
	if err != nil {
		return fmt.Errorf("open manifest: %w", err)
	}
	defer idx.Close()

	var st unix.Stat_t
	if err := unix.Stat(devicePath, &st); err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	hdr := idx.Header()
	if hdr.Major != uint32(unix.Major(uint64(st.Rdev))) || hdr.Minor != uint32(unix.Minor(uint64(st.Rdev))) {
		return fmt.Errorf("precondition: device identity mismatch")
	}

	fSrc, err := blockio.Open(devicePath, cfg.ODirect, false)
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	defer fSrc.Close()

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
		fSrc, err = blockio.Open(devicePath, false, false)
		if err != nil {
			return fmt.Errorf("open device: %w", err)
		}
		defer fSrc.Close()
	}
	tasks := make(chan job, workers)
	errCh := make(chan error, 1)
	var mismatches int64
	hash, err := digestFunc(cfg)
	if err != nil {
		return err
	}
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
