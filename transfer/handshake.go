package transfer

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/config"
)

func composeHandshake(cfg *config.Config, mode string) common.Handshake {
	hs := common.Handshake{
		Compress:      cfg.Compress,
		CompressLevel: cfg.CompressLevel,
		BlockSize:     cfg.BlockSize,
		DedupMode:     cfg.DedupMode,
		ResumeToken:   cfg.ResumeState,
		ODirect:       cfg.ODirect,
		Endianness:    common.NativeEndianness(),
	}
	switch mode {
	case StrategyChecksum:
		hs.Checksum = true
	case StrategyChecksum + "-dedup":
		hs.Checksum = true
		hs.ChecksumDedup = true
	}
	return hs
}

func setupOutput(cfg *config.Config, out io.Writer, handshake string, logger *zap.Logger) (io.WriteCloser, *bufio.Writer, error) {
	if err := common.WriteHandshake(out, composeHandshake(cfg, handshake)); err != nil {
		return nil, nil, err
	}
	return prepareOutputWriter(out, cfg, logger)
}

func prepareParallelHandshake(cfg *config.Config) string {
	mode := ""
	if cfg.VerifyChecksum {
		mode = StrategyChecksum
	}
	hs := composeHandshake(cfg, mode)
	var sb strings.Builder
	// WriteHandshake always appends a newline; trim for reuse.
	if err := common.WriteHandshake(&sb, hs); err != nil {
		return ""
	}
	return strings.TrimSpace(sb.String())
}

func writeParallelHandshake(cfg *config.Config, out io.Writer) error {
	_, err := fmt.Fprintln(out, prepareParallelHandshake(cfg))
	return err
}

func readAndValidateHandshake(bufReader *bufio.Reader, cfg *config.Config, dedup DeduplicationStrategy, verify bool) (common.Handshake, error) {
	hs, err := common.ReadHandshake(bufReader)
	if err != nil {
		return common.Handshake{}, fmt.Errorf("failed to read protocol handshake: %w", err)
	}
	if verify && !hs.Checksum {
		return hs, fmt.Errorf("unexpected protocol handshake: %s", hs.String())
	}
	if dedup != nil && !hs.ChecksumDedup {
		return hs, fmt.Errorf("unexpected protocol handshake: %s", hs.String())
	}

	if hs.Endianness != "" && hs.Endianness != common.NativeEndianness() {
		return hs, fmt.Errorf("endianness mismatch: %s", hs.Endianness)
	}
	if cfg.BlockSize > 0 && hs.BlockSize != 0 && hs.BlockSize != cfg.BlockSize {
		return hs, fmt.Errorf("block size mismatch: %d", hs.BlockSize)
	}
	if cfg.DedupMode != "" && hs.DedupMode != "" && hs.DedupMode != cfg.DedupMode {
		return hs, fmt.Errorf("dedup mode mismatch: %s", hs.DedupMode)
	}
	if cfg.ODirect && !hs.ODirect {
		return hs, fmt.Errorf("remote lacks O_DIRECT support")
	}
	if cfg.Compress != "" && hs.Compress != cfg.Compress {
		return hs, fmt.Errorf("compression mismatch: %s", hs.Compress)
	}
	if cfg.CompressLevel != 0 && hs.CompressLevel != 0 && hs.CompressLevel != cfg.CompressLevel {
		return hs, fmt.Errorf("compression level mismatch: %d", hs.CompressLevel)
	}
	return hs, nil
}
