package transfer

import "time"

// resumeChunk stores the boundaries and digest of the last processed chunk.
type resumeChunk struct {
	Chunk  [32]byte
	Offset uint64
	Length uint32
}

// resumeChunks groups the last processed chunks for each deduplication mode.
type resumeChunks struct {
	Fixed  resumeChunk
	CDC    resumeChunk
	Hybrid resumeChunk
}

// resumeCheckpoint represents the last processed chunk recorded on disk.
type resumeCheckpoint struct {
	resumeChunks
	DedupMode string
}

// resumeTracker tracks checkpoint progress for an ongoing transfer.
type resumeTracker struct {
	bytes int64
	last  time.Time
	resumeChunks
	sizeBytes uint64
	deviceID  string
	epoch     uint64
}

func (rt *resumeTracker) chunk(mode string) *resumeChunk {
	switch mode {
	case "cdc":
		return &rt.CDC
	case "hybrid":
		return &rt.Hybrid
	default:
		return &rt.Fixed
	}
}

func (rc resumeCheckpoint) chunk(mode string) resumeChunk {
	switch mode {
	case "cdc":
		return rc.CDC
	case "hybrid":
		return rc.Hybrid
	default:
		return rc.Fixed
	}
}
