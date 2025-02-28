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

	"go.uber.org/zap"
)

var Logger *zap.Logger

func SetLogger(logger *zap.Logger) {
	Logger = logger
}

func DumpChangesSequential(snapshot, source string, out io.Writer, verbose bool, useZeroCopy bool, verifyChecksum bool, compress string, compressLevel int, speedLimit int, resumeState string, parallel int) error {
	metadataDevice := GetMetadataDevice(snapshot)
	if metadataDevice == "" {
		return fmt.Errorf("failed to determine metadata device from snapshot %s", snapshot)
	}
	chunkSize, err := ReadMetadataHeader(metadataDevice)
	if err != nil {
		return fmt.Errorf("error reading metadata header: %v", err)
	}
	Logger.Info("Metadata header read", zap.Int64("chunkSize", chunkSize))
	ranges, err := GetDifferences(metadataDevice, chunkSize)
	if err != nil {
		return fmt.Errorf("error getting differences: %v", err)
	}
	Logger.Info("Changed blocks determined", zap.Int("blockCount", len(ranges)))
	handshake := common.ProtocolVersion
	if verifyChecksum {
		handshake += " checksum"
	}
	fmt.Fprintln(out, handshake)
	limitedOut := WrapRateLimitedWriter(out, speedLimit)
	compWriter, err := NewCompressionWriter(limitedOut, compress, compressLevel)
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
	var totalBytes int64
	for i, r := range ranges {
		size := int(r.End - r.Start + 1)
		data, err := ReadBlockWithRetries(srcFile, r.Start, size, maxRetries, useZeroCopy)
		if err != nil {
			return fmt.Errorf("error reading block at offset %d: %v", r.Start, err)
		}
		header := make([]byte, 12)
		binary.BigEndian.PutUint64(header[0:8], uint64(r.Start))
		binary.BigEndian.PutUint32(header[8:12], uint32(size))
		if verifyChecksum {
			sum := sha256.Sum256(data)
			header = append(header, sum[:]...)
		}
		if _, err := bufOut.Write(header); err != nil {
			return fmt.Errorf("failed to write header: %v", err)
		}
		if _, err := bufOut.Write(data); err != nil {
			return fmt.Errorf("failed to write block data: %v", err)
		}
		totalBytes += int64(size)
		if resumeState != "" {
			err := os.WriteFile(resumeState, []byte(fmt.Sprintf("%d", i+1)), 0644)
			if err != nil {
				Logger.Warn("Failed to update resume state", zap.Error(err))
			}
		}
		if verbose && i > 0 && i%100 == 0 {
			elapsed := time.Since(startTime).Seconds()
			speed := float64(totalBytes) / elapsed / 1048576.0
			Logger.Info("Sequential dump progress", zap.Int("chunk", i+1), zap.Float64("MB/s", speed))
		}
	}
	if err := bufOut.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %v", err)
	}
	if err := compWriter.Close(); err != nil {
		return fmt.Errorf("failed to close compression writer: %v", err)
	}
	elapsed := time.Since(startTime).Seconds()
	Logger.Info("Sequential transfer complete", zap.Int64("bytes", totalBytes), zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytes)/elapsed/1048576.0))
	return nil
}

func DumpChangesParallel(snapshot, source string, out io.Writer, verbose bool, verifyChecksum bool, compress string, compressLevel int, speedLimit int, resumeState string, parallel int) error {
	metadataDevice := GetMetadataDevice(snapshot)
	if metadataDevice == "" {
		return fmt.Errorf("failed to determine metadata device from snapshot %s", snapshot)
	}
	chunkSize, err := ReadMetadataHeader(metadataDevice)
	if err != nil {
		return fmt.Errorf("error reading metadata header: %v", err)
	}
	Logger.Info("Metadata header read", zap.Int64("chunkSize", chunkSize))
	ranges, err := GetDifferences(metadataDevice, chunkSize)
	if err != nil {
		return fmt.Errorf("error getting differences: %v", err)
	}
	Logger.Info("Changed blocks determined", zap.Int("blockCount", len(ranges)))
	handshake := common.ProtocolVersion
	if verifyChecksum {
		handshake += " checksum"
	}
	fmt.Fprintln(out, handshake)
	limitedOut := WrapRateLimitedWriter(out, speedLimit)
	compWriter, err := NewCompressionWriter(limitedOut, compress, compressLevel)
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
	if resumeState != "" {
		data, err := os.ReadFile(resumeState)
		if err == nil {
			if val, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				resumeStart = val
				Logger.Info("Resuming from block", zap.Int("resumeStart", resumeStart))
			}
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				size := int(task.R.End - task.R.Start + 1)
				data, err := ReadBlockWithRetries(srcFile, task.R.Start, size, maxRetries, false)
				results <- &BlockResult{
					Index:  task.Index,
					Offset: uint64(task.R.Start),
					Size:   uint32(size),
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
	blockResults := make([]*BlockResult, numBlocks-resumeStart)
	count := 0
	for res := range results {
		if res.Err != nil {
			return fmt.Errorf("error in block %d: %v", res.Index, res.Err)
		}
		blockResults[res.Index-resumeStart] = res
		count++
	}
	if count != numBlocks-resumeStart {
		return fmt.Errorf("expected %d blocks, got %d", numBlocks-resumeStart, count)
	}
	startTime := time.Now()
	var totalBytes int64
	for i, res := range blockResults {
		header := make([]byte, 12)
		binary.BigEndian.PutUint64(header[0:8], res.Offset)
		binary.BigEndian.PutUint32(header[8:12], res.Size)
		if verifyChecksum {
			sum := sha256.Sum256(res.Data)
			header = append(header, sum[:]...)
		}
		if _, err := bufOut.Write(header); err != nil {
			return fmt.Errorf("failed to write header for block %d: %v", i, err)
		}
		if _, err := bufOut.Write(res.Data); err != nil {
			return fmt.Errorf("failed to write data for block %d: %v", i, err)
		}
		totalBytes += int64(res.Size)
		if resumeState != "" {
			err := os.WriteFile(resumeState, []byte(fmt.Sprintf("%d", i+resumeStart+1)), 0644)
			if err != nil {
				Logger.Warn("Failed to update resume state", zap.Error(err))
			}
		}
		if verbose && i > 0 && i%100 == 0 {
			elapsed := time.Since(startTime).Seconds()
			speed := float64(totalBytes) / elapsed / 1048576.0
			Logger.Info("Parallel dump progress", zap.Int("block", i+resumeStart+1), zap.Float64("MB/s", speed))
		}
	}
	if err := bufOut.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %v", err)
	}
	if err := compWriter.Close(); err != nil {
		return fmt.Errorf("failed to close compression writer: %v", err)
	}
	elapsed := time.Since(startTime).Seconds()
	Logger.Info("Parallel transfer complete", zap.Int64("bytes", totalBytes), zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytes)/elapsed/1048576.0))
	return nil
}

func ProcessDumpData(in io.Reader, destPath string, verbose bool, verifyChecksum bool, compress string) error {
	decReader, err := NewDecompressionReader(in, compress)
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
	verifyFlag := false
	if strings.Contains(handshake, "checksum") {
		verifyFlag = true
	}
	if handshake != common.ProtocolVersion && handshake != (common.ProtocolVersion+" checksum") {
		return fmt.Errorf("protocol mismatch: got %q, expected %q or %q", handshake, common.ProtocolVersion, common.ProtocolVersion+" checksum")
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
			Logger.Warn("Seek error", zap.Uint64("offset", offset), zap.Error(err))
			continue
		}
		if _, err := destFile.Write(data); err != nil {
			return fmt.Errorf("failed to write data at offset %d: %v", offset, err)
		}
		totalBytes += int64(chunkSize)
	}
	elapsed := time.Since(startTime).Seconds()
	Logger.Info("Applied changes", zap.Int64("bytes", totalBytes), zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytes)/elapsed/1048576.0))
	return nil
}

func RunApply(applyFile, destDevice string, verbose bool, verifyChecksum bool, compress string) error {
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
	return ProcessDumpData(in, destDevice, verbose, verifyChecksum, compress)
}
