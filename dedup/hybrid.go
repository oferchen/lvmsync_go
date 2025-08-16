package dedup

import (
	"bytes"
	"fmt"
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
	seed    uint64
	pending []Chunk
	// reusable buffer for fixed-size block reads
	buf []byte
}

// NewHybridChunker returns a HybridChunker configured with the given block
// size and CDC window parameters. A seed may optionally be supplied for
// deterministic chunking.
func NewHybridChunker(fixed, min, avg, max int, seeds ...uint64) (*HybridChunker, error) {
	if fixed <= 0 || min <= 0 || avg <= 0 || max <= 0 {
		return nil, fmt.Errorf("sizes must be positive: fixed=%d min=%d avg=%d max=%d", fixed, min, avg, max)
	}
	if min > avg || avg > max {
		return nil, fmt.Errorf("sizes must satisfy min ≤ avg ≤ max: min=%d avg=%d max=%d", min, avg, max)
	}
	h := &HybridChunker{fixed: fixed, cdcMin: min, cdcAvg: avg, cdcMax: max}
	if len(seeds) > 0 {
		h.seed = seeds[0]
	}
	return h, nil
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

	if cap(h.buf) < h.fixed {
		h.buf = make([]byte, h.fixed)
	}
	buf := h.buf[:h.fixed]
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

	chunks, err := FastCDC(bytes.NewReader(buf), h.cdcMin, h.cdcAvg, h.cdcMax, h.seed)
	if err != nil && err != io.EOF {
		return Chunk{}, err
	}
	if len(chunks) == 0 {
		return Chunk{}, io.EOF
	}
	h.pending = chunks[1:]
	return chunks[0], nil
}
