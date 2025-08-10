package dedup

import (
	"bytes"
	"io"
)

// HybridChunker first splits the input into fixed-size blocks and then
// applies content-defined chunking within each block.
// It implements the ChunkSource interface so it can be used with Replicator.
type HybridChunker struct {
	fixed   int
	cdcMin  int
	cdcAvg  int
	cdcMax  int
	pending []Chunk
}

// NewHybridChunker returns a HybridChunker configured with the given block
// size and CDC window parameters.
func NewHybridChunker(fixed, min, avg, max int) *HybridChunker {
	return &HybridChunker{fixed: fixed, cdcMin: min, cdcAvg: avg, cdcMax: max}
}

// NextChunk returns the next chunk from r. The reader is only consulted when
// no buffered CDC chunks remain. io.EOF is returned when the reader is
// exhausted.
//
//nolint:cyclop // necessary state handling
func (h *HybridChunker) NextChunk(r io.Reader) (Chunk, error) {
	if len(h.pending) > 0 {
		c := h.pending[0]
		h.pending = h.pending[1:]
		return c, nil
	}

	buf := make([]byte, h.fixed)
	n, err := io.ReadFull(r, buf)
	if err != nil {
		if err == io.ErrUnexpectedEOF || err == io.EOF {
			if n == 0 {
				return Chunk{}, io.EOF
			}
			buf = buf[:n]
		} else {
			return Chunk{}, err
		}
	} else {
		buf = buf[:n]
	}

	chunks, err := FastCDC(bytes.NewReader(buf), h.cdcMin, h.cdcAvg, h.cdcMax)
	if err != nil && err != io.EOF {
		return Chunk{}, err
	}
	if len(chunks) == 0 {
		return Chunk{}, io.EOF
	}
	h.pending = chunks[1:]
	return chunks[0], nil
}
