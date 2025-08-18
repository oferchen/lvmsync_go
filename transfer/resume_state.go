package transfer

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)

// resumeState persists transfer checkpoints allowing interrupted transfers to resume.
type resumeChunkState struct {
	Offset uint64 `json:"offset"`
	Length uint32 `json:"length"`
	Chunk  string `json:"chunk"`
}

type resumeState struct {
	Transport         string           `json:"transport"`
	Compress          string           `json:"compress"`
	ChecksumAlgorithm string           `json:"checksum_algorithm"`
	DedupMode         string           `json:"dedup_mode"`
	Fixed             resumeChunkState `json:"fixed"`
	CDC               resumeChunkState `json:"cdc"`
	Hybrid            resumeChunkState `json:"hybrid"`
	SizeBytes         uint64           `json:"size_bytes"`
	DeviceID          string           `json:"device_id"`
	Epoch             uint64           `json:"epoch"`
	FirstBlockDigest  string           `json:"first_block_digest"`
}

// writeResumeState persists resume state; logger must be non-nil.
func writeResumeState(cfg *config.Config, logger *zap.Logger, path string, chunks resumeChunks, size uint64, deviceID string, epoch uint64) {
	toState := func(ch resumeChunk) resumeChunkState {
		return resumeChunkState{
			Offset: ch.Offset,
			Length: ch.Length,
			Chunk:  hex.EncodeToString(ch.Chunk[:]),
		}
	}
	rs := resumeState{
		Transport:         cfg.Transport,
		Compress:          cfg.Compress,
		ChecksumAlgorithm: cfg.ChecksumAlgorithm,
		DedupMode:         cfg.DedupMode,
		Fixed:             toState(chunks.Fixed),
		CDC:               toState(chunks.CDC),
		Hybrid:            toState(chunks.Hybrid),
		SizeBytes:         size,
		DeviceID:          deviceID,
		Epoch:             epoch,
		FirstBlockDigest:  cfg.FirstBlockDigest,
	}
	data, err := json.Marshal(rs)
	if err != nil {
		logger.Warn("Failed to marshal resume state", zap.Error(err))
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		logger.Warn("Failed to update resume state", zap.Error(err))
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
	rc := rt.chunk(cfg.DedupMode)
	*rc = resumeChunk{Chunk: chunk, Offset: offset, Length: uint32(size)}
	if (cfg.CheckpointBytes > 0 && rt.bytes >= int64(cfg.CheckpointBytes)) ||
		(cfg.CheckpointInterval > 0 && time.Since(rt.last) >= cfg.CheckpointInterval) {
		writeResumeState(cfg, logger, cfg.ResumeState, resumeChunks{Fixed: rt.Fixed, CDC: rt.CDC, Hybrid: rt.Hybrid}, rt.sizeBytes, rt.deviceID, rt.epoch)
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
		logger.Warn("Failed to remove resume state", zap.Error(err))
	}
	rt.bytes = 0
	rt.last = time.Time{}
	rt.resumeChunks = resumeChunks{}
	rt.sizeBytes = 0
	rt.deviceID = ""
	rt.epoch = 0
}

func readResumeState(cfg *config.Config, logger *zap.Logger, size uint64, deviceID string, epoch uint64, digest [32]byte) resumeCheckpoint {
	var out resumeCheckpoint
	if cfg.ResumeState == "" {
		return out
	}
	if strings.ToLower(cfg.VerifyLevel) != "none" {
		cfg.ResumeVerify = true
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
	if (rs.DeviceID != "" && deviceID != "" && rs.DeviceID != deviceID) ||
		(rs.SizeBytes != 0 && size != 0 && rs.SizeBytes != size) ||
		(rs.Epoch != 0 && epoch != 0 && rs.Epoch != epoch) {
		return out
	}
	if rs.FirstBlockDigest != "" && digest != ([32]byte{}) {
		b, err := hex.DecodeString(rs.FirstBlockDigest)
		if err != nil || len(b) != 32 || !bytes.Equal(b, digest[:]) {
			return out
		}
	}
	decode := func(cs resumeChunkState) (resumeChunk, bool) {
		b, err := hex.DecodeString(cs.Chunk)
		if err != nil || len(b) != 32 {
			return resumeChunk{}, false
		}
		var ch resumeChunk
		copy(ch.Chunk[:], b)
		ch.Offset = cs.Offset
		ch.Length = cs.Length
		return ch, true
	}
	if ch, ok := decode(rs.Fixed); ok {
		out.Fixed = ch
	}
	if ch, ok := decode(rs.CDC); ok {
		out.CDC = ch
	}
	if ch, ok := decode(rs.Hybrid); ok {
		out.Hybrid = ch
	}
	rc := out.chunk(cfg.DedupMode)
	if rc != (resumeChunk{}) {
		logger.Info("Resuming from chunk",
			zap.String("resume_chunk", hex.EncodeToString(rc.Chunk[:])),
			zap.Uint64("resume_offset_bytes", rc.Offset),
			zap.Uint32("resume_length_bytes", rc.Length))
	}
	return out
}
