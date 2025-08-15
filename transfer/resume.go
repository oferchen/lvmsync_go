package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/config"
)

var (
	resumeMu     sync.Mutex
	resumeBytes  int64
	resumeLast   time.Time
	resumeChunk  [32]byte
	resumeOffset uint64
	resumeLength uint32
)

type resumeState struct {
	Transport         string `json:"transport"`
	Compress          string `json:"compress"`
	ChecksumAlgorithm string `json:"checksum_algorithm"`
	DedupMode         string `json:"dedup_mode"`
	Offset            uint64 `json:"offset"`
	Length            uint32 `json:"length"`
	Chunk             string `json:"chunk"`
}

type resumeCheckpoint struct {
	Chunk  [32]byte
	Offset uint64
	Length uint32
}

func writeResumeState(cfg *config.Config, logger *zap.Logger, path string, offset uint64, length uint32, chunk [32]byte) {
	rs := resumeState{
		Transport:         cfg.Transport,
		Compress:          cfg.Compress,
		ChecksumAlgorithm: cfg.ChecksumAlgorithm,
		DedupMode:         cfg.DedupMode,
		Offset:            offset,
		Length:            length,
		Chunk:             hex.EncodeToString(chunk[:]),
	}
	data, err := json.Marshal(rs)
	if err != nil {
		if logger != nil {
			logger.Warn("Failed to marshal resume state", zap.Error(err))
		}
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		if logger != nil {
			logger.Warn("Failed to update resume state", zap.Error(err))
		}
	}
}

func saveResumeState(cfg *config.Config, offset uint64, chunk [32]byte, size int64, logger *zap.Logger) {
	if cfg.ResumeState == "" {
		return
	}
	resumeMu.Lock()
	defer resumeMu.Unlock()
	if resumeLast.IsZero() {
		resumeLast = time.Now()
	}
	resumeBytes += size
	resumeChunk = chunk
	resumeOffset = offset
	resumeLength = uint32(size)
	if (cfg.CheckpointBytes > 0 && resumeBytes >= int64(cfg.CheckpointBytes)) ||
		(cfg.CheckpointInterval > 0 && time.Since(resumeLast) >= cfg.CheckpointInterval) {
		writeResumeState(cfg, logger, cfg.ResumeState, resumeOffset, resumeLength, resumeChunk)
		resumeBytes = 0
		resumeLast = time.Now()
	}
}

func finalizeResumeState(cfg *config.Config, logger *zap.Logger) {
	if cfg.ResumeState == "" {
		return
	}
	resumeMu.Lock()
	defer resumeMu.Unlock()
	if resumeLast.IsZero() {
		return
	}
	if err := os.Remove(cfg.ResumeState); err != nil && !os.IsNotExist(err) {
		if logger != nil {
			logger.Warn("Failed to remove resume state", zap.Error(err))
		}
	}
	resumeBytes = 0
	resumeLast = time.Time{}
}

func readResumeState(cfg *config.Config, logger *zap.Logger) resumeCheckpoint {
	var out resumeCheckpoint
	if cfg.ResumeState == "" {
		return out
	}
	data, err := os.ReadFile(cfg.ResumeState)
	if err != nil {
		return out
	}
	var rs resumeState
	if err := json.Unmarshal(data, &rs); err != nil {
		return out
	}
	if rs.Transport != cfg.Transport || rs.Compress != cfg.Compress ||
		rs.ChecksumAlgorithm != cfg.ChecksumAlgorithm || rs.DedupMode != cfg.DedupMode {
		return out
	}
	b, err := hex.DecodeString(rs.Chunk)
	if err != nil || len(b) != 32 {
		return out
	}
	copy(out.Chunk[:], b)
	out.Offset = rs.Offset
	out.Length = rs.Length
	if logger != nil {
		logger.Info("Resuming from chunk",
			zap.String("resume_chunk", hex.EncodeToString(out.Chunk[:])),
			zap.Uint64("resume_offset", out.Offset),
			zap.Uint32("resume_length", out.Length))
	}
	return out
}

func findResumeIndex(cfg *config.Config, srcFile *os.File, ranges []Range, chk resumeCheckpoint, logger *zap.Logger) int {
	if cfg.ResumeState == "" {
		return 0
	}
	if cfg.DedupMode == "cdc" || cfg.DedupMode == "hybrid" {
		next := chk.Offset + uint64(chk.Length)
		for i := range ranges {
			if next < ranges[i].Start {
				return i
			}
			if next <= ranges[i].End {
				ranges[i].Start = next
				if logger != nil {
					logger.Info("Resuming after offset", zap.Uint64("resume_offset", next))
				}
				return i
			}
		}
		return len(ranges)
	}
	if chk.Chunk == [32]byte{} {
		return 0
	}
	for i, r := range ranges {
		offset, _, err := validateOffsetAndSize(r.Start, cfg.BlockSize)
		if err != nil {
			return 0
		}
		data, err := ReadBlockWithRetries(cfg, srcFile, offset, cfg.ZeroCopy, [2]int{-1, -1}, logger)
		if err != nil {
			continue
		}
		var sum [32]byte
		if cfg.ChecksumAlgorithm == "sha256" {
			sumArr := sha256.Sum256(data)
			sum = [32]byte(sumArr)
		} else {
			sum = blake3.Sum256(data)
		}
		putBlockBuffer(data)
		if sum == chk.Chunk {
			if logger != nil {
				logger.Info("Resuming after index", zap.Int("resume_index", i+1))
			}
			return i + 1
		}
	}
	return 0
}
