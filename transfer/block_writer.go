package transfer

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/internal/sizeparse"
)

// blockWriter streams block records to a destination file, optionally
// verifying checksums and syncing data at configured intervals.
type blockWriter struct {
	cfg       *config.Config
	dest      *os.File
	dedup     DeduplicationStrategy
	verify    bool
	checksum  ChecksumStrategy
	logger    *zap.Logger
	sinceSync int64
	rt        *resumeTracker
}

// newBlockWriter constructs a blockWriter, detecting the destination's physical
// block size when the configured block size is "auto" (0) and parsing the
// sync interval when provided as a string. It validates that the configured
// block size is aligned to the underlying sector size.
func newBlockWriter(cfg *config.Config, dest *os.File, dedup DeduplicationStrategy, verify bool, checksum ChecksumStrategy, logger *zap.Logger) (*blockWriter, error) {
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
		if val, _, err := sizeparse.Parse(cfg.SyncInterval); err == nil {
			cfg.SyncIntervalBytes = int(val)
		}
	}
	return &blockWriter{
		cfg:      cfg,
		dest:     dest,
		dedup:    dedup,
		verify:   verify,
		checksum: checksum,
		logger:   logger,
	}, nil
}

// write consumes block records from reader and writes them to the destination
// file. It applies sparse punching for zero ranges, performs optional
// checksum verification, and issues fdatasync calls after each configured
// interval. It returns the total number of bytes written.
func (bw *blockWriter) write(reader *bufio.Reader) (int64, error) {
	headerLen := 16
	if bw.verify {
		headerLen += bw.checksum.Size()
	}
	headerBuf := make([]byte, headerLen)
	var total int64
	for {
		offset, chunkSize, crc, transmitted, err := readBlockHeader(reader, headerBuf, bw.verify, bw.checksum)
		if err == io.EOF {
			break
		}
		if err != nil {
			return total, err
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
		written, err := processBlock(bw.cfg, bw.dest, bw.dedup, bw.verify, bw.checksum, offset, crc, transmitted, data, chunkSize, bw.logger)
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
		saveResumeState(bw.cfg, bw.rt, offset, chunkID, int64(chunkSize), bw.logger)
		if written {
			total += int64(chunkSize)
			bw.sinceSync += int64(chunkSize)
			if bw.cfg.SyncIntervalBytes > 0 && bw.sinceSync >= int64(bw.cfg.SyncIntervalBytes) {
				if err := fdatasyncFile(bw.dest); err != nil {
					return total, err
				}
				bw.sinceSync = 0
			}
		}
	}
	return total, nil
}
