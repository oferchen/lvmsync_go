// Package root contains the top-level command wiring and helpers.
package root

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pierrec/lz4/v4"
	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/config"
	"github.com/oferchen/lvmsync_go/internal/cpufeatures"
	"github.com/oferchen/lvmsync_go/manifest"
)

type compressionInfo struct {
	ChunkSize int    `json:"chunk_size"`
	Algorithm string `json:"algorithm"`
	Level     int    `json:"level"`
}

type planOutput struct {
	Config         *config.Config             `json:"config"`
	TransportOrder []string                   `json:"transport_order"`
	EstimatedBytes int64                      `json:"estimated_bytes"`
	Compression    map[string]compressionInfo `json:"compression"`
}

const (
	lz4MaxChunk   = 256 * 1024
	defaultZstdLv = 1
	maxAutoZstdLv = 3
)

func redactConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	rc := *cfg
	rc.SSHPassword = ""
	rc.SSHKeyPath = ""
	rc.SSHHostKey = ""
	rc.SSHHostKeyPath = ""
	rc.KnownHosts = ""
	rc.ClientCert = ""
	rc.ClientKey = ""
	rc.CACert = ""
	return &rc
}

func emitPlan(cfg *config.Config, args []string, logger *zap.Logger) error {
	if len(args) < 1 {
		return fmt.Errorf("missing source argument")
	}
	if cfg.AllowInsecure || strings.Contains(cfg.Transport, "rsync") {
		fmt.Fprintln(os.Stderr, "allow_insecure enabled; security checks disabled")
		if !cfg.AllowInsecure {
			return fmt.Errorf("insecure configuration requires --allow-insecure")
		}
	}
	_, est, err := estimateBytes(args[0], cfg)
	if err != nil {
		return err
	}
	po := planOutput{
		Config:         redactConfig(cfg),
		TransportOrder: splitList(cfg.Transport),
		EstimatedBytes: est,
		Compression:    buildCompressionPlan(cfg),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(po)
}

func estimateBytes(src string, cfg *config.Config) (int64, int64, error) {
	info, err := os.Stat(src)
	if err != nil {
		return 0, 0, fmt.Errorf("stat source: %w", err)
	}
	size := info.Size()
	if cfg.ManifestPath == "" {
		return size, size, nil
	}
	idx, err := manifest.Open(cfg.ManifestPath)
	if err != nil {
		return 0, 0, fmt.Errorf("open manifest: %w", err)
	}
	defer idx.Close()
	f, err := os.Open(src)
	if err != nil {
		return 0, 0, fmt.Errorf("open source: %w", err)
	}
	defer f.Close()
	chunks := idx.ChunkCount()
	samples := chunks
	if samples > 100 {
		samples = 100
	}
	step := uint64(1)
	if samples > 0 && chunks > samples {
		step = chunks / samples
	}
	changed := 0
	for i := uint64(0); i < samples; i++ {
		idxPos := i * step
		if idxPos >= chunks {
			idxPos = chunks - 1
		}
		off, length, flags, _, _, err := idx.Entry(idxPos)
		if err != nil {
			return 0, 0, fmt.Errorf("manifest entry: %w", err)
		}
		buf := make([]byte, length)
		n, err := f.ReadAt(buf, int64(off))
		if err != nil && err != io.EOF {
			return 0, 0, fmt.Errorf("read source: %w", err)
		}
		data := buf[:n]
		xx := xxh3.Hash(data)
		digest := blake3.Sum256(data)
		if !idx.Match(off, uint32(n), flags, xx, func() [32]byte { return digest }) {
			changed++
		}
		if err == io.EOF {
			break
		}
	}
	ratio := float64(changed)
	if samples > 0 {
		ratio /= float64(samples)
	}
	est := int64(ratio * float64(size))
	return size, est, nil
}

func buildCompressionPlan(cfg *config.Config) map[string]compressionInfo {
	lz4Level := int(lz4.Level1)
	if strings.ToLower(cfg.LZ4Level) == "hc" {
		lz4Level = int(lz4.Level9)
	}
	zstdLevel := cfg.ZstdLevel
	out := make(map[string]compressionInfo)
	if cfg.DedupMode == "cdc" || cfg.DedupMode == "hybrid" {
		sizes := map[string]int{"min": cfg.CDCMin, "avg": cfg.CDCAvg, "max": cfg.CDCMax}
		for k, v := range sizes {
			if v > 0 {
				algo, lvl := selectAlgorithmPlan(v, cfg.Compress, lz4Level, zstdLevel)
				out[k] = compressionInfo{ChunkSize: v, Algorithm: algo, Level: lvl}
			}
		}
	} else if cfg.BlockSize > 0 {
		algo, lvl := selectAlgorithmPlan(cfg.BlockSize, cfg.Compress, lz4Level, zstdLevel)
		out["block"] = compressionInfo{ChunkSize: cfg.BlockSize, Algorithm: algo, Level: lvl}
	}
	return out
}

func selectAlgorithmPlan(chunkLen int, compress string, lz4Level, zstdLevel int) (string, int) {
	if compress != "auto" && compress != "" {
		switch compress {
		case "lz4":
			if lz4Level == 0 {
				lz4Level = int(lz4.Level1)
			}
			return "lz4", lz4Level
		case "zstd":
			if zstdLevel == 0 {
				zstdLevel = defaultZstdLv
			}
			return "zstd", zstdLevel
		default:
			return compress, lz4Level
		}
	}
	if chunkLen < lz4MaxChunk {
		if lz4Level == 0 {
			lz4Level = int(lz4.Level1)
		}
		return "lz4", lz4Level
	}
	if cpufeatures.HasAVX2() || cpufeatures.HasNEON() {
		if zstdLevel <= 0 {
			zstdLevel = defaultZstdLv
		} else if zstdLevel > maxAutoZstdLv {
			zstdLevel = maxAutoZstdLv
		}
		return "zstd", zstdLevel
	}
	if lz4Level == 0 {
		lz4Level = int(lz4.Level1)
	}
	return "lz4", lz4Level
}

func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
