package transfer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

func prepareOutputWriter(out io.Writer, cfg *config.Config, logger *zap.Logger) (io.WriteCloser, *bufio.Writer, error) {
	_ = logger
	limitedOut := WrapRateLimitedWriter(out, cfg.SpeedLimit)
	compWriter, err := NewCompressionWriter(limitedOut, cfg.Compress, cfg.CompressLevel, cfg.CompressConcurrency)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create compression writer: %w", err)
	}
	bufOut := bufio.NewWriter(compWriter)
	return compWriter, bufOut, nil
}

// cleanupOutput flushes and closes writers; logger must be non-nil.
func cleanupOutput(buf *bufio.Writer, w io.WriteCloser, logger *zap.Logger) {
	if err := buf.Flush(); err != nil {
		logger.Warn("Failed to flush output", zap.Error(err))
	}
	if err := w.Close(); err != nil {
		logger.Warn("Failed to close writer", zap.Error(err))
	}
}

func readBlockHeader(reader *bufio.Reader, headerBuf []byte, verify bool, checksum ChecksumStrategy) (uint64, uint32, []byte, error) {
	_, err := io.ReadFull(reader, headerBuf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return 0, 0, nil, io.EOF
	}
	if err != nil {
		return 0, 0, nil, fmt.Errorf("failed to read chunk header: %w", err)
	}

	offset := binary.BigEndian.Uint64(headerBuf[0:8])
	if offset > math.MaxInt64 {
		return 0, 0, nil, fmt.Errorf("offset %d overflows int64", offset)
	}
	chunkSize := binary.BigEndian.Uint32(headerBuf[8:12])

	var transmittedSum []byte
	if verify {
		transmittedSum = make([]byte, checksum.Size())
		copy(transmittedSum, headerBuf[12:])
	}

	return offset, chunkSize, transmittedSum, nil
}

func readBlockData(cfg *config.Config, reader io.Reader, chunkSize uint32) ([]byte, error) {
	if chunkSize == 0 {
		return nil, nil
	}
	size := int(chunkSize)
	var data []byte
	if cfg.ODirect {
		data = getAlignedBlockBuffer(size)
	} else {
		data = getBlockBuffer(size)
	}
	if _, err := io.ReadFull(reader, data); err != nil {
		if cfg.ODirect {
			putAlignedBlockBuffer(data)
		} else {
			putBlockBuffer(data)
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read chunk data: %w", err)
	}
	return data, nil
}

func verifyChecksum(verify bool, checksum ChecksumStrategy, data, transmitted []byte, offset uint64) error {
	if !verify {
		return nil
	}
	computed := checksum.Compute(data)
	if !bytes.Equal(transmitted, computed) {
		return fmt.Errorf("checksum mismatch at offset %d", offset)
	}
	return nil
}

// writeData seeks to offset and writes the provided data to destFile.
// If seeking fails, no write occurs and the seek error is returned.
// writeData writes data at offset; logger must be non-nil.
func writeData(destFile *os.File, offset uint64, data []byte, logger *zap.Logger) error {
	if offset > math.MaxInt64 {
		return fmt.Errorf("offset %d overflows int64", offset)
	}
	if _, err := destFile.Seek(int64(offset), io.SeekStart); err != nil {
		logger.Warn("Seek error", zap.Uint64("offset", offset), zap.Error(err))
		return fmt.Errorf("failed to seek to offset %d: %w", offset, err)
	}
	if _, err := destFile.Write(data); err != nil {
		return fmt.Errorf("failed to write data at offset %d: %w", offset, err)
	}
	return nil
}

// processBlock validates and writes a block to destFile.
// It returns whether data was written. Seek or write failures
// from writeData are propagated to the caller.
func processBlock(
	cfg *config.Config,
	destFile *os.File,
	dedup DeduplicationStrategy,
	verify bool,
	checksum ChecksumStrategy,
	offset uint64,
	transmitted, data []byte,
	chunkSize uint32,
	logger *zap.Logger,
) (bool, error) {
	if offset > math.MaxInt64 {
		return false, fmt.Errorf("offset %d overflows int64", offset)
	}
	if err := verifyChecksum(verify, checksum, data, transmitted, offset); err != nil {
		return false, err
	}
	if chunkSize == 0 || isAllZero(data) {
		if err := punchHole(destFile, offset, cfg.BlockSize); err != nil {
			zero := getAlignedBlockBuffer(cfg.BlockSize)
			if err := writeData(destFile, offset, zero, logger); err != nil {
				putAlignedBlockBuffer(zero)
				return false, err
			}
			putAlignedBlockBuffer(zero)
		}
		return true, nil
	}
	if dedup != nil {
		intOffset := int64(offset)
		if !dedup.ShouldTransfer(intOffset, data) {
			return false, nil
		}
		dedup.RecordTransfer(intOffset, data)
	}
	if err := writeData(destFile, offset, data, logger); err != nil {
		return false, err
	}
	return true, nil
}
