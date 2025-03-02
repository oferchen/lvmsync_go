// transfer/transfer.go
package transfer

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"lvmsync_go/common"
	"lvmsync_go/config"

	"go.uber.org/zap"
)

var Logger *zap.Logger

func SetLogger(logger *zap.Logger) {
	Logger = logger
}

func DumpChangesSequential(cfg *config.Config, snapshot, source string, out io.Writer) error {
	metadataDevice := GetMetadataDevice(snapshot)
	if metadataDevice == "" {
		return fmt.Errorf("failed to determine metadata device from snapshot %s", snapshot)
	}
	blockSize := int64(cfg.BlockSize)

	Logger.Info("Using configured block size", zap.Int("blockSize", cfg.BlockSize))

	ranges, err := GetDifferences(metadataDevice, blockSize)
	if err != nil {
		return fmt.Errorf("error getting differences: %v", err)
	}
	Logger.Info("Changed blocks determined", zap.Int("blockCount", len(ranges)))

	var totalDataSize int64
	for _, r := range ranges {
		totalDataSize += (r.End - r.Start + 1)
	}

	handshake := common.ProtocolVersion
	if cfg.VerifyChecksum {
		handshake += " checksum"
	}
	fmt.Fprintln(out, handshake)

	limitedOut := WrapRateLimitedWriter(out, cfg.SpeedLimit)
	compWriter, err := NewCompressionWriter(limitedOut, cfg.Compress, cfg.CompressLevel)
	if err != nil {
		return fmt.Errorf("failed to create compression writer: %v", err)
	}
	bufOut := bufio.NewWriter(compWriter)

	srcFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open source device %s: %v", source, err)
	}
	defer srcFile.Close()

	startTime := time.Now()
	var totalBytesTransferred int64
	for i, r := range ranges {
		data, err := ReadBlockWithRetries(cfg, srcFile, r.Start, cfg.ZeroCopy)
		if err != nil {
			return fmt.Errorf("error reading block at offset %d: %v", r.Start, err)
		}

		header := make([]byte, 12)
		binary.BigEndian.PutUint64(header[0:8], uint64(r.Start))
		binary.BigEndian.PutUint32(header[8:12], uint32(cfg.BlockSize))
		if cfg.VerifyChecksum {
			sum := sha256.Sum256(data)
			header = append(header, sum[:]...)
		}

		if _, err := bufOut.Write(header); err != nil {
			return fmt.Errorf("failed to write header: %v", err)
		}
		if _, err := bufOut.Write(data); err != nil {
			return fmt.Errorf("failed to write block data: %v", err)
		}

		totalBytesTransferred += int64(cfg.BlockSize)

		if cfg.ResumeState != "" {
			err := os.WriteFile(cfg.ResumeState, []byte(fmt.Sprintf("%d", i+1)), 0644)
			if err != nil {
				Logger.Warn("Failed to update resume state", zap.Error(err))
			}
		}

		if cfg.Progress {
			progressPercent := float64(totalBytesTransferred) / float64(totalDataSize) * 100.0
			fmt.Fprintf(os.Stderr, "\rProgress: %.2f%%", progressPercent)
		}

		if cfg.Verbose > 0 && i > 0 && i%100 == 0 {
			elapsed := time.Since(startTime).Seconds()
			speed := float64(totalBytesTransferred) / elapsed / 1048576.0
			Logger.Info("Sequential dump progress", zap.Int("chunk", i+1), zap.Float64("MB/s", speed))
		}
	}
	if cfg.Progress {
		fmt.Fprintln(os.Stderr, "")
	}

	if err := bufOut.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %v", err)
	}
	if err := compWriter.Close(); err != nil {
		return fmt.Errorf("failed to close compression writer: %v", err)
	}

	elapsed := time.Since(startTime).Seconds()
	Logger.Info("Sequential transfer complete", zap.Int64("bytes", totalBytesTransferred), zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytesTransferred)/elapsed/1048576.0))
	return nil
}

func DumpChangesParallel(cfg *config.Config, snapshot, source string, out io.Writer) error {
	if cfg.ZeroCopy {
		Logger.Warn("ZeroCopy mode enabled, falling back to sequential execution")
		return DumpChangesSequential(cfg, snapshot, source, out)
	}

	metadataDevice := GetMetadataDevice(snapshot)
	if metadataDevice == "" {
		return fmt.Errorf("failed to determine metadata device from snapshot %s", snapshot)
	}
	blockSize := int64(cfg.BlockSize)

	Logger.Info("Using configured block size", zap.Int("blockSize", cfg.BlockSize))

	ranges, err := GetDifferences(metadataDevice, blockSize)
	if err != nil {
		return fmt.Errorf("error getting differences: %v", err)
	}
	Logger.Info("Changed blocks determined", zap.Int("blockCount", len(ranges)))

	var totalDataSize int64
	for _, r := range ranges {
		totalDataSize += (r.End - r.Start + 1)
	}

	handshake := common.ProtocolVersion
	if cfg.VerifyChecksum {
		handshake += " checksum"
	}
	fmt.Fprintln(out, handshake)

	limitedOut := WrapRateLimitedWriter(out, cfg.SpeedLimit)
	compWriter, err := NewCompressionWriter(limitedOut, cfg.Compress, cfg.CompressLevel)
	if err != nil {
		return fmt.Errorf("failed to create compression writer: %v", err)
	}
	bufOut := bufio.NewWriter(compWriter)

	srcFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open source device %s: %v", source, err)
	}
	defer srcFile.Close()

	numBlocks := len(ranges)
	tasks := make(chan BlockTask, numBlocks)
	results := make(chan *BlockResult, numBlocks)
	resumeStart := 0

	if cfg.ResumeState != "" {
		data, err := os.ReadFile(cfg.ResumeState)
		if err == nil {
			if val, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				resumeStart = val
				Logger.Info("Resuming from block", zap.Int("resumeStart", resumeStart))
			}
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < cfg.Parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				data, err := ReadBlockWithRetries(cfg, srcFile, task.R.Start, false)
				results <- &BlockResult{
					Index:  task.Index,
					Offset: uint64(task.R.Start),
					Size:   uint32(cfg.BlockSize),
					Data:   data,
					Err:    err,
				}
			}
		}()
	}

	for i := resumeStart; i < numBlocks; i++ {
		tasks <- BlockTask{Index: i, R: ranges[i]}
	}
	close(tasks)

	go func() {
		wg.Wait()
		close(results)
	}()

	startTime := time.Now()
	var totalBytesTransferred int64

	for res := range results {
		if res.Err != nil {
			return fmt.Errorf("error in block %d: %v", res.Index, res.Err)
		}

		header := make([]byte, 12)
		binary.BigEndian.PutUint64(header[0:8], res.Offset)
		binary.BigEndian.PutUint32(header[8:12], res.Size)
		if cfg.VerifyChecksum {
			sum := sha256.Sum256(res.Data)
			header = append(header, sum[:]...)
		}

		if _, err := bufOut.Write(header); err != nil {
			return fmt.Errorf("failed to write header: %v", err)
		}
		if _, err := bufOut.Write(res.Data); err != nil {
			return fmt.Errorf("failed to write data: %v", err)
		}

		totalBytesTransferred += int64(res.Size)

		if cfg.ResumeState != "" {
			err := os.WriteFile(cfg.ResumeState, []byte(fmt.Sprintf("%d", res.Index+1)), 0644)
			if err != nil {
				Logger.Warn("Failed to update resume state", zap.Error(err))
			}
		}

		if cfg.Progress {
			progressPercent := float64(totalBytesTransferred) / float64(totalDataSize) * 100.0
			fmt.Fprintf(os.Stderr, "\rProgress: %.2f%%", progressPercent)
		}

		if cfg.Verbose > 0 && res.Index > 0 && res.Index%100 == 0 {
			elapsed := time.Since(startTime).Seconds()
			speed := float64(totalBytesTransferred) / elapsed / 1048576.0
			Logger.Info("Parallel dump progress", zap.Int("block", res.Index+1), zap.Float64("MB/s", speed))
		}
	}

	if cfg.Progress {
		fmt.Fprintln(os.Stderr, "")
	}

	bufOut.Flush()
	compWriter.Close()

	elapsed := time.Since(startTime).Seconds()
	Logger.Info("Parallel transfer complete", zap.Int64("bytes", totalBytesTransferred), zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytesTransferred)/elapsed/1048576.0))
	return nil
}

func ProcessDumpData(cfg *config.Config, in io.Reader, destPath string) error {
	decReader, err := NewDecompressionReader(in, cfg.Compress)
	if err != nil {
		return fmt.Errorf("failed to create decompression reader: %v", err)
	}
	defer decReader.Close()

	reader := bufio.NewReader(decReader)

	handshake, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read protocol handshake: %v", err)
	}
	handshake = strings.TrimSpace(handshake)
	verifyFlag := strings.Contains(handshake, "checksum")

	if handshake != common.ProtocolVersion && handshake != (common.ProtocolVersion+" checksum") {
		return fmt.Errorf("protocol mismatch: got %q, expected %q or %q",
			handshake, common.ProtocolVersion, common.ProtocolVersion+" checksum")
	}

	destFile, err := os.OpenFile(destPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open destination device %s: %v", destPath, err)
	}
	defer destFile.Close()

	startTime := time.Now()
	var totalBytes int64
	headerLen := 12
	if verifyFlag {
		headerLen += 32
	}
	headerBuf := make([]byte, headerLen)

	for {
		_, err := io.ReadFull(reader, headerBuf)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read chunk header: %v", err)
		}

		offset := binary.BigEndian.Uint64(headerBuf[0:8])
		chunkSize := binary.BigEndian.Uint32(headerBuf[8:12])

		var transmittedSum [32]byte
		if verifyFlag {
			copy(transmittedSum[:], headerBuf[12:44])
		}

		data := make([]byte, chunkSize)
		if _, err := io.ReadFull(reader, data); err != nil {
			return fmt.Errorf("failed to read chunk data: %v", err)
		}

		if verifyFlag {
			computed := sha256.Sum256(data)
			if transmittedSum != computed {
				return fmt.Errorf("checksum mismatch at offset %d", offset)
			}
		}

		if _, err := destFile.Seek(int64(offset), os.SEEK_SET); err != nil {
			zap.L().Warn("Seek error", zap.Uint64("offset", offset), zap.Error(err))
			continue
		}
		if _, err := destFile.Write(data); err != nil {
			return fmt.Errorf("failed to write data at offset %d: %v", offset, err)
		}

		totalBytes += int64(chunkSize)
	}

	elapsed := time.Since(startTime).Seconds()
	zap.L().Info("Applied changes",
		zap.Int64("bytes", totalBytes),
		zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytes)/elapsed/1048576.0),
	)

	return nil
}

func RunApply(cfg *config.Config, applyFile, destDevice string) error {
	var in io.Reader
	if applyFile == "-" {
		in = os.Stdin
	} else {
		f, err := os.Open(applyFile)
		if err != nil {
			return fmt.Errorf("failed to open apply file %s: %v", applyFile, err)
		}
		defer f.Close()
		in = f
	}

	return ProcessDumpData(cfg, in, destDevice)
}
