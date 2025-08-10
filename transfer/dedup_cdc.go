package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"io"
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/zeebo/blake3"

	"lvmsync_go/config"
	"lvmsync_go/dedup"
)

// CDCDedup implements a simple FastCDC based deduplication helper. It
// chunks data using the dedup.Chunker and records per chunk hashes in a
// Bloom filter and an in-memory index. Each chunk is hashed with BLAKE3
// while a final SHA-256 digest is computed over the chunk digests.
type CDCDedup struct {
	chunker   *dedup.Chunker
	bloom     *bloom.BloomFilter
	index     map[[32]byte]struct{}
	stateFile string

	mu  sync.Mutex
	sha hash.Hash
}

// NewCDCDedup constructs a CDCDedup using the tunables provided in cfg.
func NewCDCDedup(cfg *config.Config) *CDCDedup {
	bf := bloom.NewWithEstimates(uint(cfg.BloomEntries), cfg.BloomFpRate)
	return &CDCDedup{
		chunker:   dedup.NewChunker(cfg.CDCMin, cfg.CDCAvg, cfg.CDCMax),
		bloom:     bf,
		index:     make(map[[32]byte]struct{}),
		stateFile: cfg.DedupStateFile,
		sha:       sha256.New(),
	}
}

// ChunkAndHash splits p into FastCDC chunks recording hashes. The returned
// slice contains all detected chunks. The second return value is the final
// SHA-256 of the concatenated chunk digests.
func (c *CDCDedup) ChunkAndHash(p []byte) ([]dedup.Chunk, [32]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rdr := bytes.NewReader(p)
	var out []dedup.Chunk
	var offset int64
	for {
		ch, err := c.chunker.NextChunk(rdr)
		if err == io.EOF && ch.Length == 0 {
			break
		}
		if err != nil && err != io.EOF {
			return nil, [32]byte{}, err
		}
		digest := blake3.Sum256(ch.Data)
		if !c.bloom.Test(digest[:]) {
			c.bloom.Add(digest[:])
			c.index[digest] = struct{}{}
		}
		c.sha.Write(digest[:])
		ch.Offset = offset
		out = append(out, ch)
		offset += int64(ch.Length)
		if err == io.EOF {
			break
		}
	}
	var final [32]byte
	copy(final[:], c.sha.Sum(nil))
	c.sha.Reset()
	return out, final, nil
}

// SaveState persists the seen chunk hashes to the configured state file.
// The state format is a simple binary concatenation of 32 byte digests.
func (c *CDCDedup) SaveState() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return saveStateFile(c.stateFile, func(w io.Writer) error {
		for h := range c.index {
			if err := binary.Write(w, binary.LittleEndian, h); err != nil {
				return err
			}
		}
		return nil
	})
}
