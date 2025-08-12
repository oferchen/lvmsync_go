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
	hs := common.Handshake{Compress: cfg.Compress}
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
	htokens := []string{common.ProtocolVersion}
	if cfg.VerifyChecksum {
		htokens = append(htokens, StrategyChecksum)
	}
	htokens = append(htokens, "compress:"+cfg.Compress)
	return strings.Join(htokens, " ")
}

func writeParallelHandshake(cfg *config.Config, out io.Writer) error {
	_, err := fmt.Fprintln(out, prepareParallelHandshake(cfg))
	return err
}

func readAndValidateHandshake(bufReader *bufio.Reader, dedup DeduplicationStrategy, verify bool) (common.Handshake, error) {
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
	return hs, nil
}
