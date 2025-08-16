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
	comps := splitCompression(cfg.Compress)
	hs := common.Handshake{
		Transports:  splitList(cfg.Transport),
		Compress:    comps[0],
		Compressors: comps,
		Digests:     splitDigests(cfg.ChecksumAlgorithm),
		CDCMin:      cfg.CDCMin,
		CDCAvg:      cfg.CDCAvg,
		CDCMax:      cfg.CDCMax,
		ResumeToken: cfg.ResumeToken,
		MaxInFlight: cfg.Concurrency,
		CRC32C:      true,
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
	htokens := strings.Fields(strings.TrimSpace(sb.String()))
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
	if !hs.CRC32C {
		return hs, fmt.Errorf("remote lacks CRC32C support")
	}
	transport, err := negotiate(splitList(cfg.Transport), hs.Transports)
	if err == nil && transport != "" {
		cfg.Transport = transport
		hs.Transports = []string{transport}
	}
	compressList := hs.Compressors
	if len(compressList) == 0 && hs.Compress != "" {
		compressList = []string{hs.Compress}
	}
	compress, err := negotiate(splitCompression(cfg.Compress), compressList)
	if err != nil {
		return hs, fmt.Errorf("no common compression algorithm")
	}
	cfg.Compress = compress
	hs.Compress = compress
	hs.Compressors = []string{compress}
	digest, err := negotiate(splitDigests(cfg.ChecksumAlgorithm), hs.Digests)
	if err != nil {
		return hs, fmt.Errorf("no common digest algorithm")
	}
	cfg.ChecksumAlgorithm = digest
	hs.Digests = []string{digest}

	if hs.Endianness != "" && hs.Endianness != common.NativeEndianness() {
		return hs, fmt.Errorf("endianness mismatch: %s", hs.Endianness)
	}
	if cfg.BlockSize > 0 && hs.BlockSize != 0 && hs.BlockSize != cfg.BlockSize {
		return hs, fmt.Errorf("block size mismatch: %d", hs.BlockSize)
	}
	if cfg.DedupMode != "" && hs.DedupMode != "" && hs.DedupMode != cfg.DedupMode {
		return hs, fmt.Errorf("dedup mode mismatch: %s", hs.DedupMode)
	}
	if cfg.CDCMin > 0 && hs.CDCMin > 0 && hs.CDCMin != cfg.CDCMin {
		return hs, fmt.Errorf("cdc min mismatch: %d", hs.CDCMin)
	}
	if hs.CDCMin > 0 {
		cfg.CDCMin = hs.CDCMin
	}
	if cfg.CDCAvg > 0 && hs.CDCAvg > 0 && hs.CDCAvg != cfg.CDCAvg {
		return hs, fmt.Errorf("cdc avg mismatch: %d", hs.CDCAvg)
	}
	if hs.CDCAvg > 0 {
		cfg.CDCAvg = hs.CDCAvg
	}
	if cfg.CDCMax > 0 && hs.CDCMax > 0 && hs.CDCMax != cfg.CDCMax {
		return hs, fmt.Errorf("cdc max mismatch: %d", hs.CDCMax)
	}
	if hs.CDCMax > 0 {
		cfg.CDCMax = hs.CDCMax
	}
	if cfg.ResumeToken != "" && hs.ResumeToken != "" && hs.ResumeToken != cfg.ResumeToken {
		return hs, fmt.Errorf("resume token mismatch: %s", hs.ResumeToken)
	}
	if hs.ResumeToken != "" {
		cfg.ResumeToken = hs.ResumeToken
	}
	if cfg.Concurrency > 0 && hs.MaxInFlight > 0 && hs.MaxInFlight != cfg.Concurrency {
		return hs, fmt.Errorf("max in-flight mismatch: %d", hs.MaxInFlight)
	}
	if hs.MaxInFlight > 0 {
		cfg.Concurrency = hs.MaxInFlight
	}
	if cfg.ODirect && !hs.ODirect {
		return hs, fmt.Errorf("remote lacks O_DIRECT support")
	}
	if cfg.CompressLevel != 0 && hs.CompressLevel != 0 && hs.CompressLevel != cfg.CompressLevel {
		return hs, fmt.Errorf("compression level mismatch: %d", hs.CompressLevel)
	}
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
