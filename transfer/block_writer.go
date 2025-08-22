package transfer

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/config"
	"github.com/oferchen/lvmsync_go/internal/sizeparse"
)

// blockWriter streams block records to a destination file, optionally
// verifying checksums and syncing data at configured intervals.
type blockWriter struct {
	cfg       *config.Config
	dest      *os.File
	dedup     DeduplicationStrategy
	intra     *chunkCache
	verify    bool
	checksum  ChecksumStrategy
	logger    *zap.Logger
	sinceSync int64
	rt        *resumeTracker
	deps      *Deps
	wal       *WAL
	applied   []Range
}

// newBlockWriter constructs a blockWriter, detecting the destination's physical
// block size when the configured block size is "auto" (0) and parsing the
// sync interval when provided as a string. It validates that the configured
// block size is aligned to the underlying sector size.
func newBlockWriter(cfg *config.Config, dest *os.File, dedup DeduplicationStrategy, verify bool, checksum ChecksumStrategy, logger *zap.Logger, wal *WAL, applied []Range) (*blockWriter, error) {
	return newBlockWriterWithDeps(cfg, dest, dedup, verify, checksum, logger, wal, applied, DefaultDeps)
}

func newBlockWriterWithDeps(cfg *config.Config, dest *os.File, dedup DeduplicationStrategy, verify bool, checksum ChecksumStrategy, logger *zap.Logger, wal *WAL, applied []Range, deps *Deps) (*blockWriter, error) {
	if cfg.BlockSize <= 0 || cfg.ODirect {
		sector, err := DetectSectorSize(dest)
		if err != nil {
			return nil, fmt.Errorf("detect sector size: %w", err)
		}
		if cfg.BlockSize <= 0 {
			cfg.BlockSize = sector
		} else if cfg.ODirect && cfg.BlockSize%sector != 0 {
			return nil, fmt.Errorf("block size %d not multiple of sector %d", cfg.BlockSize, sector)
		}
	}
	if cfg.SyncIntervalBytes == 0 && cfg.SyncInterval != "" {
		val, isPercent, err := sizeparse.Parse(cfg.SyncInterval)
		if err != nil || isPercent {
			return nil, fmt.Errorf("invalid sync interval %q: %w", cfg.SyncInterval, err)
		}
		if val > uint64(math.MaxInt) {
			return nil, fmt.Errorf("sync interval %q overflows int", cfg.SyncInterval)
		}
		cfg.SyncIntervalBytes = int(val)
	}
	bw := &blockWriter{
		cfg:      cfg,
		dest:     dest,
		dedup:    dedup,
		verify:   verify,
		checksum: checksum,
		logger:   logger,
		deps:     deps,
		wal:      wal,
		applied:  applied,
	}
	if cfg.IntraDedup {
		bw.intra = newChunkCache(intraCacheCapacity)
	}
	return bw, nil
}

func containsRange(rs []Range, start, end uint64) bool {
	for _, r := range rs {
		if start >= r.Start && end <= r.End {
			return true
		}
	}
	return false
}

// write consumes block records from reader and writes them to the destination
// file. It punches sparse holes for zero ranges when cfg.Sparse != "never",
// performs optional
// checksum verification, and issues fdatasync calls after each configured
// interval. It returns the total number of bytes written.
func (bw *blockWriter) write(reader *bufio.Reader) (int64, error) {
	headerLen := 16
	if bw.verify {
		headerLen += bw.checksum.Size()
	}
	headerBuf := make([]byte, headerLen)
	var total int64
	var zeroStart uint64
	var zeroLen uint64
	var holes []Range
	for {
		offset, chunkSize, crc, transmitted, err := readBlockHeader(reader, headerBuf, bw.verify, bw.checksum)
		if err == io.EOF {
			break
		}
		if err != nil {
			return total, err
		}
		end := offset + uint64(chunkSize)
		if chunkSize == 0 {
			end = offset + uint64(bw.cfg.BlockSize)
		}
		if containsRange(bw.applied, offset, end) {
			if chunkSize > 0 {
				if _, err := io.CopyN(io.Discard, reader, int64(chunkSize)); err != nil {
					return total, err
				}
			}
			continue
		}
		data, err := readBlockData(bw.cfg, reader, chunkSize)
		if err == io.EOF {
			break
		}
		if err != nil {
			return total, err
		}
		var chunkID [32]byte
		if data != nil {
			chunkID = blake3.Sum256(data)
		} else {
			chunkID = zeroHash(int(chunkSize))
		}
		written, zlen, err := processBlock(bw.cfg, bw.dest, bw.dedup, bw.intra, bw.verify, bw.checksum, offset, crc, transmitted, data, chunkSize, bw.logger, bw.wal)
		if bw.cfg.ODirect {
			if data != nil {
				putAlignedBlockBuffer(data)
			}
		} else if data != nil {
			putBlockBuffer(data)
		}
		if err != nil {
			return total, err
		}
		if zlen > 0 {
			if zeroLen == 0 {
				zeroStart = offset
				zeroLen = zlen
			} else if offset == zeroStart+zeroLen {
				zeroLen += zlen
			} else {
				if bw.wal != nil {
					r := Range{Start: zeroStart, End: zeroStart + zeroLen}
					if err := bw.wal.Append(r); err != nil {
						return total, err
					}
					bw.applied = append(bw.applied, r)
				}
				if err := writeZeroRange(bw.cfg, bw.dest, zeroStart, zeroLen, bw.logger, bw.deps); err != nil {
					return total, err
				}
				holes = append(holes, Range{Start: zeroStart, End: zeroStart + zeroLen})
				total += int64(zeroLen)
				bw.sinceSync += int64(zeroLen)
				if bw.cfg.SyncIntervalBytes > 0 && bw.sinceSync >= int64(bw.cfg.SyncIntervalBytes) {
					if err := bw.deps.FdatasyncFile(bw.dest); err != nil {
						return total, err
					}
					bw.sinceSync = 0
				}
				zeroStart = offset
				zeroLen = zlen
			}
			saveResumeState(bw.cfg, bw.rt, offset, chunkID, int64(zlen), bw.logger)
			continue
		}
		if zeroLen > 0 {
			if bw.wal != nil {
				r := Range{Start: zeroStart, End: zeroStart + zeroLen}
				if err := bw.wal.Append(r); err != nil {
					return total, err
				}
				bw.applied = append(bw.applied, r)
			}
			if err := writeZeroRange(bw.cfg, bw.dest, zeroStart, zeroLen, bw.logger, bw.deps); err != nil {
				return total, err
			}
			holes = append(holes, Range{Start: zeroStart, End: zeroStart + zeroLen})
			total += int64(zeroLen)
			bw.sinceSync += int64(zeroLen)
			if bw.cfg.SyncIntervalBytes > 0 && bw.sinceSync >= int64(bw.cfg.SyncIntervalBytes) {
				if err := bw.deps.FdatasyncFile(bw.dest); err != nil {
					return total, err
				}
				bw.sinceSync = 0
			}
			zeroLen = 0
		}
		saveResumeState(bw.cfg, bw.rt, offset, chunkID, int64(chunkSize), bw.logger)
		if written {
			if bw.wal != nil {
				r := Range{Start: offset, End: offset + uint64(chunkSize)}
				bw.applied = append(bw.applied, r)
			}
			total += int64(chunkSize)
			bw.sinceSync += int64(chunkSize)
			if bw.cfg.SyncIntervalBytes > 0 && bw.sinceSync >= int64(bw.cfg.SyncIntervalBytes) {
				if err := bw.deps.FdatasyncFile(bw.dest); err != nil {
					return total, err
				}
				bw.sinceSync = 0
			}
		}
	}
	if zeroLen > 0 {
		if bw.wal != nil {
			if err := bw.wal.Append(Range{Start: zeroStart, End: zeroStart + zeroLen}); err != nil {
				return total, err
			}
		}
		if err := writeZeroRange(bw.cfg, bw.dest, zeroStart, zeroLen, bw.logger, bw.deps); err != nil {
			return total, err
		}
		holes = append(holes, Range{Start: zeroStart, End: zeroStart + zeroLen})
		total += int64(zeroLen)
		bw.sinceSync += int64(zeroLen)
		if bw.cfg.SyncIntervalBytes > 0 && bw.sinceSync >= int64(bw.cfg.SyncIntervalBytes) {
			if err := bw.deps.FdatasyncFile(bw.dest); err != nil {
				return total, err
			}
			bw.sinceSync = 0
		}
	}
	if bw.cfg.Sparse != "never" && seekHoleSupported(bw.dest) {
		for _, h := range holes {
			off, err := nextDataOffset(bw.dest, int64(h.Start))
			if err != nil {
				return total, err
			}
			if off != -1 && off < int64(h.End) {
				return total, fmt.Errorf("hole verification failed at offset %d", h.Start)
			}
		}
	}
	return total, nil
}
