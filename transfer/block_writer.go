package transfer

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/config"
)

func validateOffsetAndSize(offset uint64, size int) (int64, uint32, error) {
	if offset > math.MaxInt64 {
		return 0, 0, fmt.Errorf("offset %d overflows int64", offset)
	}
	if size < 0 || size > int(math.MaxUint32) {
		return 0, 0, fmt.Errorf("invalid block size %d: must be between 0 and %d", size, uint32(math.MaxUint32))
	}
	return int64(offset), uint32(size), nil
}

func iterateBlocks(cfg *config.Config, ranges []Range, srcFile *os.File, bufOut *bufio.Writer, dedup DeduplicationStrategy, pipeFds [2]int, logger *zap.Logger) (int64, int, *Manifest, error) {
	var totalBytes int64
	skippedBlocks := 0
	var header [12]byte
	manifest := &Manifest{}
	h := newDigestHasher(cfg.ChecksumAlgorithm)
	for _, r := range ranges {
		offset, blockSize, err := validateOffsetAndSize(r.Start, cfg.BlockSize)
		if err != nil {
			return totalBytes, skippedBlocks, manifest, err
		}
		data, err := ReadBlockWithRetries(cfg, srcFile, offset, cfg.ZeroCopy, pipeFds, logger)
		if err != nil {
			return totalBytes, skippedBlocks, manifest, fmt.Errorf("error reading block at offset %d: %w", r.Start, err)
		}
		if dedup != nil {
			if !dedup.ShouldTransfer(offset, data) {
				skippedBlocks++
				putBlockBuffer(data)
				continue
			}
			dedup.RecordTransfer(offset, data)
		}

		binary.BigEndian.PutUint64(header[0:8], r.Start)
		if isAllZero(data) {
			binary.BigEndian.PutUint32(header[8:12], 0)
			if _, err := bufOut.Write(header[:]); err != nil {
				putBlockBuffer(data)
				return totalBytes, skippedBlocks, manifest, fmt.Errorf("failed to write header: %w", err)
			}
			saveResumeState(cfg, [32]byte{}, int64(blockSize), logger)
			putBlockBuffer(data)
			totalBytes += int64(blockSize)
			continue
		}
		binary.BigEndian.PutUint32(header[8:12], blockSize)
		if _, err := bufOut.Write(header[:]); err != nil {
			putBlockBuffer(data)
			return totalBytes, skippedBlocks, manifest, fmt.Errorf("failed to write header: %w", err)
		}
		if _, err := bufOut.Write(data); err != nil {
			putBlockBuffer(data)
			return totalBytes, skippedBlocks, manifest, fmt.Errorf("failed to write block data: %w", err)
		}

		h.Write(data)
		sum := blake3.Sum256(data)
		manifest.Append(sum, int64(r.Start), int(blockSize))
		saveResumeState(cfg, sum, int64(blockSize), logger)

		putBlockBuffer(data)

		totalBytes += int64(blockSize)
	}
	manifest.DigestAlgo = cfg.ChecksumAlgorithm
	manifest.FinalDigest = h.Sum(nil)
	return totalBytes, skippedBlocks, manifest, nil
}

func prepareResultHeader(cfg *config.Config, checksum ChecksumStrategy, res *BlockResult, header []byte) int {
	binary.BigEndian.PutUint64(header[0:8], res.Offset)
	binary.BigEndian.PutUint32(header[8:12], res.Size)
	n := 12
	if cfg.VerifyChecksum {
		sum := checksum.Compute(res.Data)
		copy(header[12:], sum)
		n += checksum.Size()
	}
	return n
}

func writeResult(bufOut *bufio.Writer, header, data []byte) error {
	if _, err := bufOut.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := bufOut.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}
	return nil
}

func processParallelResults(
	cfg *config.Config,
	results <-chan *BlockResult,
	bufOut *bufio.Writer,
	checksum ChecksumStrategy,
	totalDataSize int64,
	startTime time.Time,
	logger *zap.Logger,
) (int64, *Manifest, error) {
	headerSize := 12
	if cfg.VerifyChecksum {
		headerSize += checksum.Size()
	}
	header := make([]byte, headerSize)
	var totalBytesTransferred int64
	h := newDigestHasher(cfg.ChecksumAlgorithm)
	manifest := &Manifest{}
	for res := range results {
		if res.Err != nil {
			return totalBytesTransferred, manifest, fmt.Errorf("error in block %d: %w", res.Index, res.Err)
		}

		n := prepareResultHeader(cfg, checksum, res, header)
		if err := writeResult(bufOut, header[:n], res.Data); err != nil {
			if res.Data != nil {
				putBlockBuffer(res.Data)
			}
			return totalBytesTransferred, manifest, err
		}
		if res.Data != nil {
			h.Write(res.Data)
			manifest.Append(res.ChunkID, int64(res.Offset), int(res.Size))
			saveResumeState(cfg, res.ChunkID, int64(res.Size), logger)
			putBlockBuffer(res.Data)
		} else {
			saveResumeState(cfg, res.ChunkID, 0, logger)
		}

		totalBytesTransferred += int64(res.Size)
		reportProgress(cfg, totalBytesTransferred, totalDataSize, res.Index, startTime, logger)
	}
	manifest.DigestAlgo = cfg.ChecksumAlgorithm
	manifest.FinalDigest = h.Sum(nil)
	return totalBytesTransferred, manifest, nil
}

func worker(cfg *config.Config, srcFile *os.File, tasks <-chan BlockTask, results chan<- *BlockResult, wg *sync.WaitGroup, logger *zap.Logger) {
	defer wg.Done()
	unlock := pinWorkerToDevice(cfg, srcFile, logger)
	defer unlock()
	for task := range tasks {
		offset, blockSize, err := validateOffsetAndSize(task.R.Start, cfg.BlockSize)
		if err != nil {
			results <- &BlockResult{Index: task.Index, Err: err}
			continue
		}
		data, err := ReadBlockWithRetries(cfg, srcFile, offset, false, [2]int{-1, -1}, logger)
		zero := err == nil && isAllZero(data)
		var resData []byte
		size := blockSize
		var chunkID [32]byte
		if zero {
			putBlockBuffer(data)
			resData = nil
			size = 0
		} else {
			resData = data
			chunkID = blake3.Sum256(data)
		}
		results <- &BlockResult{
			Index:   task.Index,
			Offset:  task.R.Start,
			Size:    size,
			Data:    resData,
			ChunkID: chunkID,
			Err:     err,
		}
	}
}

func finalizeResults(wg *sync.WaitGroup, results chan<- *BlockResult) {
	wg.Wait()
	close(results)
}
