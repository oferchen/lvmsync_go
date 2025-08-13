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
		return verifyWithManifest(src, dst, manifest, logger)
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

func verifyWithManifest(src, dst, manifest string, logger *zap.Logger) error {
	data, err := os.ReadFile(manifest)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	m, err := transfer.UnmarshalManifest(data)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
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

	mismatches := 0
	for _, ch := range m.Chunks {
		bufSrc := make([]byte, ch.Length)
		if _, err := fSrc.ReadAt(bufSrc, ch.Offset); err != nil {
			return fmt.Errorf("read source: %w", err)
		}
		bufDst := make([]byte, ch.Length)
		if _, err := fDst.ReadAt(bufDst, ch.Offset); err != nil {
			return fmt.Errorf("read dest: %w", err)
		}
		if blake3.Sum256(bufSrc) != blake3.Sum256(bufDst) {
			mismatches++
			logger.Error("mismatched_block", zap.Int64("offset_bytes", ch.Offset))
		}
	}
	if mismatches > 0 {
		return fmt.Errorf("%d blocks differ", mismatches)
	}
	logger.Info("verification complete")
	return nil
}
