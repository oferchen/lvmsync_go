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
		Transports: splitList(cfg.Transport),
		Compress:   splitCompression(cfg.Compress),
		Digests:    splitDigests(cfg.ChecksumAlgorithm),
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
	htokens := []string{common.ProtocolVersion}
	if cfg.VerifyChecksum {
		htokens = append(htokens, StrategyChecksum)
	}
	htokens = append(htokens, "compress:"+strings.Join(splitCompression(cfg.Compress), ","))
	htokens = append(htokens, "digest:"+strings.Join(splitDigests(cfg.ChecksumAlgorithm), ","))
	return strings.Join(htokens, " ")
}

func writeParallelHandshake(cfg *config.Config, out io.Writer) error {
	_, err := fmt.Fprintln(out, prepareParallelHandshake(cfg))
	return err
}

func readAndValidateHandshake(cfg *config.Config, bufReader *bufio.Reader, dedup DeduplicationStrategy, verify bool) (common.Handshake, error) {
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
	transport, err := negotiate(splitList(cfg.Transport), hs.Transports)
	if err == nil && transport != "" {
		cfg.Transport = transport
		hs.Transports = []string{transport}
	}
	compress, err := negotiate(splitCompression(cfg.Compress), hs.Compress)
	if err != nil {
		return hs, fmt.Errorf("no common compression algorithm")
	}
	cfg.Compress = compress
	hs.Compress = []string{compress}
	digest, err := negotiate(splitDigests(cfg.ChecksumAlgorithm), hs.Digests)
	if err != nil {
		return hs, fmt.Errorf("no common digest algorithm")
	}
	cfg.ChecksumAlgorithm = digest
	hs.Digests = []string{digest}
	return hs, nil
}

func splitList(s string) []string {
	return commonSplit(s)
}

func splitCompression(s string) []string {
	if s == StrategyAuto || s == "" {
		return []string{"zstd", "lz4"}
	}
	return commonSplit(s)
}

func splitDigests(s string) []string {
	if s == "" {
		return []string{"blake3", "sha256"}
	}
	return commonSplit(s)
}

func commonSplit(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func negotiate(preferred, supported []string) (string, error) {
	for _, p := range preferred {
		for _, s := range supported {
			if p == s {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("no match")
}
