package verify

import (
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
	"lvmsync_go/config"
	"lvmsync_go/manifest"
	"lvmsync_go/transfer"
)

func init() {
	rootcmd.RunVerify = Run
}

// Run executes the verify command with the provided arguments and logger.
// Args should exclude the "verify" subcommand itself.
func Run(args []string, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
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
			flagSets := config.NewFlagSets(defaults)
			fs := pflag.NewFlagSet("verify", pflag.ContinueOnError)
			cfg, remaining, err := config.LoadConfig(flagSets, defaults, fs, argv)
			if err != nil {
				return err
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
			return verifyDevices(cfg, remaining[0], remaining[1], cfg.ManifestPath, logger)
		},
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

func verifyDevices(cfg *config.Config, src, dst, manifest string, logger *zap.Logger) error {
	if manifest != "" {
		return verifyWithManifest(cfg, src, manifest, logger)
	}
	return verifyFull(cfg, src, dst, logger)
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

func verifyWithManifest(cfg *config.Config, src, manifestPath string, logger *zap.Logger) error {
	idx, err := manifest.Open(manifestPath)
	if err != nil {
		return fmt.Errorf("open manifest: %w", err)
	}
	defer idx.Close()
	fSrc, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer fSrc.Close()

	mismatches := 0
	buf := make([]byte, 0)
	hash := digestFunc(cfg)
	for i := uint64(0); i < idx.ChunkCount(); i++ {
		off, length, _, digest, err := idx.Entry(i)
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
