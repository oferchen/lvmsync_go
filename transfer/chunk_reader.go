package transfer

import (
	"fmt"
	"io"
	"os"
)

const defaultChunkSize = 8 * 1024 * 1024

// ChunkReader streams fixed-size chunks from a device, skipping
// indices already confirmed in the provided bitmap.
// Each bit in the bitmap corresponds to a chunk index. If the bit is 1,
// the chunk is considered confirmed and skipped.
type ChunkReader struct {
	f         *os.File
	blockSize int
	bitmap    []byte
	index     int64
}

// NewChunkReader opens path using O_DIRECT when available and returns a ChunkReader.
// blockSize defaults to 8MiB when set to 0. The caller must Close the reader.
func NewChunkReader(path string, blockSize int, bitmap []byte) (*ChunkReader, error) {
	if blockSize == 0 {
		blockSize = defaultChunkSize
	}
	f, _, err := openFileODirect(path, os.O_RDONLY)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	return &ChunkReader{f: f, blockSize: blockSize, bitmap: bitmap}, nil
}

// Next reads the next unconfirmed chunk. It returns io.EOF when no data remains.
// Returned buffers are page-aligned when O_DIRECT is in effect. Callers should
// return the buffer via putAlignedBlockBuffer.
func (cr *ChunkReader) Next() (int64, []byte, error) {
	for {
		if cr.bitmap != nil {
			byteIdx := cr.index / 8
			bit := cr.index % 8
			if int(byteIdx) < len(cr.bitmap) && cr.bitmap[byteIdx]&(1<<uint(bit)) != 0 {
				cr.index++
				continue
			}
		}
		offset := cr.index * int64(cr.blockSize)
		buf := getAlignedBlockBuffer(cr.blockSize)
		n, err := cr.f.ReadAt(buf, offset)
		if err == io.EOF && n == 0 {
			putAlignedBlockBuffer(buf)
			return 0, nil, io.EOF
		}
		if err != nil && err != io.EOF {
			putAlignedBlockBuffer(buf)
			return 0, nil, err
		}
		if n != cr.blockSize {
			putAlignedBlockBuffer(buf)
			return 0, nil, fmt.Errorf("short read: expected %d, got %d", cr.blockSize, n)
		}
		cr.index++
		return offset, buf, nil
	}
}

// Close releases underlying resources.
func (cr *ChunkReader) Close() error {
	if cr.f != nil {
		return cr.f.Close()
	}
	return nil
}

// Confirm marks the given chunk index as confirmed in the bitmap.
// If the bitmap is nil or too small, it is resized.
func (cr *ChunkReader) Confirm(index int64) {
	byteIdx := index / 8
	bit := index % 8
	if int(byteIdx) >= len(cr.bitmap) {
		newSize := byteIdx + 1
		nb := make([]byte, newSize)
		copy(nb, cr.bitmap)
		cr.bitmap = nb
	}
	cr.bitmap[byteIdx] |= 1 << uint(bit)
}
