package transfer

import "time"

// resumeChunk stores the boundaries and digest of the last processed chunk.
type resumeChunk struct {
	Chunk  [32]byte
	Offset uint64
	Length uint32
}

// resumeCheckpoint represents the last processed chunk recorded on disk.
type resumeCheckpoint struct {
	resumeChunk
}

// resumeTracker tracks checkpoint progress for an ongoing transfer.
type resumeTracker struct {
	bytes int64
	last  time.Time
	resumeChunk
}
