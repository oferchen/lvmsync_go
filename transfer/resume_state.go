package transfer

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

// resumeState persists transfer checkpoints allowing interrupted transfers to resume.
type resumeState struct {
	Transport         string `json:"transport"`
	Compress          string `json:"compress"`
	ChecksumAlgorithm string `json:"checksum_algorithm"`
	DedupMode         string `json:"dedup_mode"`
	Offset            uint64 `json:"offset"`
	Length            uint32 `json:"length"`
	Chunk             string `json:"chunk"`
}

// resumeCheckpoint represents the last processed chunk on disk.
type resumeCheckpoint struct {
	Chunk  [32]byte
	Offset uint64
	Length uint32
}

// resumeTracker tracks checkpoint progress for an ongoing transfer.
type resumeTracker struct {
	bytes  int64
	last   time.Time
	chunk  [32]byte
	offset uint64
	length uint32
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

func saveResumeState(cfg *config.Config, rt *resumeTracker, offset uint64, chunk [32]byte, size int64, logger *zap.Logger) {
	if cfg.ResumeState == "" || rt == nil {
		return
	}
	if rt.last.IsZero() {
		rt.last = time.Now()
	}
	rt.bytes += size
	rt.chunk = chunk
	rt.offset = offset
	rt.length = uint32(size)
	if (cfg.CheckpointBytes > 0 && rt.bytes >= int64(cfg.CheckpointBytes)) ||
		(cfg.CheckpointInterval > 0 && time.Since(rt.last) >= cfg.CheckpointInterval) {
		writeResumeState(cfg, logger, cfg.ResumeState, rt.offset, rt.length, rt.chunk)
		rt.bytes = 0
		rt.last = time.Now()
	}
}

func finalizeResumeState(cfg *config.Config, rt *resumeTracker, logger *zap.Logger) {
	if cfg.ResumeState == "" || rt == nil {
		return
	}
	if rt.last.IsZero() {
		return
	}
	if err := os.Remove(cfg.ResumeState); err != nil && !os.IsNotExist(err) {
		if logger != nil {
			logger.Warn("Failed to remove resume state", zap.Error(err))
		}
	}
	rt.bytes = 0
	rt.last = time.Time{}
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
		rs.ChecksumAlgorithm != cfg.ChecksumAlgorithm {
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
