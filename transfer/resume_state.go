package transfer

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
)

func resumeWALPath(path string) string { return path + ".wal" }

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
	ResumeToken       string           `json:"resume_token"`
	Fixed             resumeChunkState `json:"fixed"`
	CDC               resumeChunkState `json:"cdc"`
	Hybrid            resumeChunkState `json:"hybrid"`
	SizeBytes         uint64           `json:"size_bytes"`
	DeviceID          string           `json:"device_id"`
	Epoch             uint64           `json:"epoch"`
	PartitionHash     string           `json:"partition_hash"`
	FirstBlockDigest  string           `json:"first_block_digest"`
}

// writeResumeState persists resume state using a WAL file. The state is written
// to a temporary file, fsynced, and atomically renamed to `<path>.wal` to avoid
// corruption on crashes. The logger must be non-nil.
func writeResumeState(cfg *config.Config, logger *zap.Logger, path string, chunks resumeChunks, size uint64, deviceID string, epoch uint64, partHash [32]byte) {
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
		ResumeToken:       cfg.ResumeToken,
		Fixed:             toState(chunks.Fixed),
		CDC:               toState(chunks.CDC),
		Hybrid:            toState(chunks.Hybrid),
		SizeBytes:         size,
		DeviceID:          deviceID,
		Epoch:             epoch,
		PartitionHash:     hex.EncodeToString(partHash[:]),
		FirstBlockDigest:  cfg.FirstBlockDigest,
	}
	data, err := json.Marshal(rs)
	if err != nil {
		logger.Warn("Failed to marshal resume state", zap.Error(err))
		return
	}

	walPath := resumeWALPath(path)
	tmpPath := walPath + ".tmp"

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		logger.Warn("Failed to create resume WAL", zap.Error(err))
		return
	}
	if _, err := f.Write(data); err != nil {
		logger.Warn("Failed to write resume WAL", zap.Error(err))
		f.Close()
		os.Remove(tmpPath)
		return
	}
	if err := f.Sync(); err != nil {
		logger.Warn("Failed to fsync resume WAL", zap.Error(err))
		f.Close()
		os.Remove(tmpPath)
		return
	}
	if err := f.Close(); err != nil {
		logger.Warn("Failed to close resume WAL", zap.Error(err))
		os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, walPath); err != nil {
		logger.Warn("Failed to rename resume WAL", zap.Error(err))
		os.Remove(tmpPath)
		return
	}
	if dir, err := os.Open(filepath.Dir(walPath)); err == nil {
		dir.Sync()
		dir.Close()
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
		writeResumeState(cfg, logger, cfg.ResumeState, resumeChunks{Fixed: rt.Fixed, CDC: rt.CDC, Hybrid: rt.Hybrid}, rt.sizeBytes, rt.deviceID, rt.epoch, rt.partitionHash)
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
	if err := os.Remove(resumeWALPath(cfg.ResumeState)); err != nil && !os.IsNotExist(err) {
		logger.Warn("Failed to remove resume WAL", zap.Error(err))
	}
	rt.bytes = 0
	rt.last = time.Time{}
	rt.resumeChunks = resumeChunks{}
	rt.sizeBytes = 0
	rt.deviceID = ""
	rt.epoch = 0
	rt.partitionHash = [32]byte{}
}

func readResumeState(cfg *config.Config, logger *zap.Logger, id device.DeviceIdentity, digest [32]byte) resumeCheckpoint {
	if cfg.ResumeState == "" {
		return resumeCheckpoint{}
	}

	if cp, ok := loadResumeState(resumeWALPath(cfg.ResumeState), cfg, id, digest, logger); ok {
		return cp
	}
	if cp, ok := loadResumeState(cfg.ResumeState, cfg, id, digest, logger); ok {
		return cp
	}
	if strings.ToLower(cfg.VerifyLevel) != "none" {
		cfg.ResumeVerify = true
	}
	return resumeCheckpoint{}
}

// loadResumeState loads and validates a resume state file. It returns the
// checkpoint and true on success.
func loadResumeState(path string, cfg *config.Config, id device.DeviceIdentity, digest [32]byte, logger *zap.Logger) (resumeCheckpoint, bool) {
	var out resumeCheckpoint
	data, err := os.ReadFile(path)
	if err != nil {
		return out, false
	}
	var rs resumeState
	if err := json.Unmarshal(data, &rs); err != nil {
		return out, false
	}
	if rs.Transport != cfg.Transport || rs.Compress != cfg.Compress || rs.ChecksumAlgorithm != cfg.ChecksumAlgorithm || rs.DedupMode != cfg.DedupMode {
		return out, false
	}
	out.DedupMode = rs.DedupMode
	if rs.ResumeToken != "" && cfg.ResumeToken != "" && rs.ResumeToken != cfg.ResumeToken {
		return out, false
	}
	if cfg.ResumeToken == "" {
		cfg.ResumeToken = rs.ResumeToken
	}
	var part [32]byte
	if b, err := hex.DecodeString(rs.PartitionHash); err == nil && len(b) == 32 {
		copy(part[:], b)
	}
	storedID := device.DeviceIdentity{SizeBytes: rs.SizeBytes, FSUUID: rs.DeviceID, ManifestEpoch: rs.Epoch, PartitionHash: part}
	actualID := id
	if storedID.SizeBytes == 0 || actualID.SizeBytes == 0 {
		storedID.SizeBytes = 0
		actualID.SizeBytes = 0
	}
	if storedID.FSUUID == "" || actualID.FSUUID == "" {
		storedID.FSUUID = ""
		actualID.FSUUID = ""
	}
	if storedID.ManifestEpoch == 0 || actualID.ManifestEpoch == 0 {
		storedID.ManifestEpoch = 0
		actualID.ManifestEpoch = 0
	}
	if storedID.PartitionHash == ([32]byte{}) || actualID.PartitionHash == ([32]byte{}) {
		storedID.PartitionHash = [32]byte{}
		actualID.PartitionHash = [32]byte{}
	}
	if !device.SameIdentityStrict(storedID, actualID) {
		return out, false
	}
	if rs.FirstBlockDigest != "" && digest != ([32]byte{}) {
		b, err := hex.DecodeString(rs.FirstBlockDigest)
		if err != nil || len(b) != 32 || !bytes.Equal(b, digest[:]) {
			return out, false
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
	return out, true
}
