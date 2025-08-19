package verify

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/internal/config"
	cpufeatures "lvmsync_go/internal/cpufeatures"
	digestpkg "lvmsync_go/internal/digest"
	manifestpkg "lvmsync_go/manifest"
	"lvmsync_go/transfer"
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
				logger.Warn(w)
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
			return r.verifyDevices(cfg, remaining[0], remaining[1], cfg.ManifestPath, logger)
		},
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

func (r *Runner) verifyDevices(cfg *config.Config, src, dst, manifestPath string, logger *zap.Logger) error {
	if manifestPath == "" {
		manifestPath = src + ".manifest"
	}
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			ctx := context.Background()
			if cfg.ManifestTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, cfg.ManifestTimeout)
				defer cancel()
			}
			hybridFixed := uint32(0)
			if cfg.DedupMode == "hybrid" {
				hybridFixed = uint32(cfg.BlockSize)
			}
			if err := r.Rebuild(ctx, src, manifestPath, logger, cfg.ManifestProgressInterval, cfg.ManifestAllowMounted, uint32(cfg.CDCMin), uint32(cfg.CDCAvg), uint32(cfg.CDCMax), hybridFixed); err != nil {
				return fmt.Errorf("rebuild manifest: %w", err)
			}
		} else {
			return fmt.Errorf("stat manifest: %w", err)
		}
	}
	if err := verifyWithManifest(cfg, dst, manifestPath, logger); err != nil {
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
	sampled := strings.ToLower(cfg.VerifyLevel) == "sampled"
	match, srcSum, dstSum, err := digestpkg.VerifyFiles(src, dst, alg, sampled)
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

func digestFunc(cfg *config.Config) func([]byte) [32]byte {
	if strings.ToLower(cfg.ChecksumAlgorithm) == "sha256" {
		return sha256.Sum256
	}
	return blake3.Sum256
}

func verifyFull(cfg *config.Config, src, dst string, logger *zap.Logger) error {
	blockSize := cfg.BlockSize
	if blockSize == 0 {
		blockSize = 8 * 1024 * 1024
	}
	fSrc, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer fSrc.Close()
	fDst, err := os.Open(dst)
	if err != nil {
		return fmt.Errorf("open dest: %w", err)
	}
	defer fDst.Close()

	infoSrc, err := fSrc.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	infoDst, err := fDst.Stat()
	if err != nil {
		return fmt.Errorf("stat dest: %w", err)
	}
	if infoSrc.Size() != infoDst.Size() {
		logger.Error("size mismatch", zap.Int64("source_bytes", infoSrc.Size()), zap.Int64("dest_bytes", infoDst.Size()))
		return fmt.Errorf("size mismatch")
	}

	total := infoSrc.Size()
	mismatches := 0
	bufSrc := make([]byte, blockSize)
	bufDst := make([]byte, blockSize)
	digest := digestFunc(cfg)
	for off := int64(0); off < total; off += int64(blockSize) {
		size := blockSize
		if remaining := int(total - off); remaining < size {
			size = remaining
		}
		if err := transfer.ReadBlockInto(fSrc, off, bufSrc[:size]); err != nil && err != io.EOF {
			return fmt.Errorf("read source: %w", err)
		}
		if err := transfer.ReadBlockInto(fDst, off, bufDst[:size]); err != nil && err != io.EOF {
			return fmt.Errorf("read dest: %w", err)
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
	fSrc, err := os.Open(devicePath)
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	defer fSrc.Close()

	mismatches := 0
	buf := make([]byte, 0)
	hash := digestFunc(cfg)
	for i := uint64(0); i < idx.ChunkCount(); i++ {
		off, length, _, _, digest, err := idx.Entry(i)
		if err != nil {
			return fmt.Errorf("manifest entry: %w", err)
		}
		if length == 0 {
			continue
		}
		if int(length) > cap(buf) {
			buf = make([]byte, int(length))
		}
		buf = buf[:int(length)]
		if _, err := fSrc.ReadAt(buf, int64(off)); err != nil {
			return fmt.Errorf("read source: %w", err)
		}
		actual := hash(buf)
		if actual != digest {
			mismatches++
			logger.Error("digest_mismatch",
				zap.Uint64("offset_bytes", off),
				zap.String("expected_digest", fmt.Sprintf("%x", digest[:])),
				zap.String("actual_digest", fmt.Sprintf("%x", actual[:])))
		}
	}
	if mismatches > 0 {
		return fmt.Errorf("%d blocks differ", mismatches)
	}
	logger.Info("verification complete")
	return nil
}
