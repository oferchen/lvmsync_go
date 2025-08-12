package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"io"
	"os"
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/zeebo/blake3"
	"golang.org/x/sys/unix"

	"lvmsync_go/config"
	"lvmsync_go/dedup"
)

// CDCDedup implements a simple FastCDC based deduplication helper. It
// chunks data using the dedup.Chunker and records per chunk hashes in a
// Bloom filter and an optional mmap backed bitset index. Each chunk is
// hashed with BLAKE3 while a final SHA-256 digest is computed over the
// chunk digests.
type CDCDedup struct {
	chunker   *dedup.Chunker
	bloom     *bloom.BloomFilter
	index     []byte
	indexFile *os.File
	indexMask uint64
	stateFile string

	mu  sync.Mutex
	sha hash.Hash
}

// NewCDCDedup constructs a CDCDedup using the tunables provided in cfg.
func NewCDCDedup(cfg *config.Config) *CDCDedup {
	bf := bloom.NewWithEstimates(uint(cfg.BloomEntries), cfg.BloomFpRate)
	cd := &CDCDedup{
		chunker:   dedup.NewChunker(cfg.CDCMin, cfg.CDCAvg, cfg.CDCMax),
		bloom:     bf,
		stateFile: cfg.DedupStateFile,
		sha:       sha256.New(),
	}
	if cfg.BloomMBits > 0 {
		size := 1 << (cfg.BloomMBits - 3)
		f, err := os.OpenFile(cfg.DedupStateFile+".idx", os.O_RDWR|os.O_CREATE, 0o600)
		if err == nil {
			if err := f.Truncate(int64(size)); err == nil {
				data, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
				if err == nil {
					cd.index = data
					cd.indexFile = f
					cd.indexMask = (1 << cfg.BloomMBits) - 1
				} else {
					f.Close()
				}
			} else {
				f.Close()
			}
		}
	}
	return cd
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
		}
		if len(c.index) > 0 {
			idx := binary.LittleEndian.Uint64(digest[:8]) & c.indexMask
			byteIdx := idx >> 3
			bit := byte(1 << (idx & 7))
			c.index[byteIdx] |= bit
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
	if err := saveStateFile(nil, c.stateFile, func(w io.Writer) error {
		_, err := c.bloom.WriteTo(w)
		return err
	}); err != nil {
		return err
	}
	if len(c.index) > 0 {
		if err := unix.Msync(c.index, unix.MS_SYNC); err != nil {
			return err
		}
		if c.indexFile != nil {
			return c.indexFile.Sync()
		}
	}
	return nil
}
