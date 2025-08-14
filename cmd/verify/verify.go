package verify

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/manifest"
	"lvmsync_go/transfer"
)

// Run executes the verify command with the provided arguments and logger.
// Args should exclude the "verify" subcommand itself.
func Run(args []string, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
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
			for _, set := range flagSets.All() {
				fs.AddFlagSet(set)
			}
			var manifest string
			fs.StringVar(&manifest, "manifest", "", "Manifest file providing range hints")
			cfg, remaining, err := config.LoadConfig(flagSets, defaults, fs, argv)
			if err != nil {
				return err
			}
			if len(remaining) != 2 {
				fs.Usage()
				return fmt.Errorf("usage: lvmsync verify [flags] <source> <dest>")
			}
			return verifyDevices(cfg, remaining[0], remaining[1], manifest, logger)
		},
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

func verifyDevices(cfg *config.Config, src, dst, manifest string, logger *zap.Logger) error {
	if manifest != "" {
		return verifyWithManifest(src, manifest, logger)
	}
	return verifyFull(cfg, src, dst, logger)
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
	for off := int64(0); off < total; off += int64(blockSize) {
		bufSrc, err := transfer.ReadBlock(fSrc, off, blockSize)
		if err != nil && err != io.EOF {
			return fmt.Errorf("read source: %w", err)
		}
		bufDst, err := transfer.ReadBlock(fDst, off, blockSize)
		if err != nil && err != io.EOF {
			return fmt.Errorf("read dest: %w", err)
		}
		if blake3.Sum256(bufSrc) != blake3.Sum256(bufDst) {
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

func verifyWithManifest(src, manifestPath string, logger *zap.Logger) error {
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
	for i := 0; i < idx.ChunkCount(); i++ {
		off, length, _, digest := idx.Entry(i)
		if length == 0 {
			continue
		}
		buf := make([]byte, length)
		if _, err := fSrc.ReadAt(buf, int64(off)); err != nil {
			return fmt.Errorf("read source: %w", err)
		}
		actual := blake3.Sum256(buf)
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
