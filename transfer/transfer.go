// transfer/transfer.go
package transfer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"lvmsync_go/common"
	"lvmsync_go/config"
	"lvmsync_go/internal/blocksize"

	"go.uber.org/zap"
)

var Logger *zap.Logger
var workerWG *sync.WaitGroup

func SetLogger(logger *zap.Logger) {
	Logger = logger
}

type ChecksumState struct {
	Checksums map[uint64][]byte
	Strategy  string
}

func LoadChecksumState(filename string) (*ChecksumState, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return &ChecksumState{Checksums: make(map[uint64][]byte), Strategy: "sha256"}, nil
		}
		return nil, err
	}
	defer file.Close()

	state := &ChecksumState{}
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(state); err != nil {
		return nil, err
	}

	if state.Checksums == nil {
		state.Checksums = make(map[uint64][]byte)
	}
	if state.Strategy == "" {
		state.Strategy = "sha256"
	}
	return state, nil
}

func SaveChecksumState(filename string, state *ChecksumState) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	return encoder.Encode(state)
}

func prepareOutputWriter(out io.Writer, cfg *config.Config) (io.WriteCloser, *bufio.Writer, error) {
	limitedOut := WrapRateLimitedWriter(out, cfg.SpeedLimit)
	compWriter, err := NewCompressionWriter(limitedOut, cfg.Compress, cfg.CompressLevel, cfg.CompressConcurrency)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create compression writer: %w", err)
	}
	bufOut := bufio.NewWriter(compWriter)
	return compWriter, bufOut, nil
}

func cleanupOutput(buf *bufio.Writer, w io.WriteCloser) {
	if err := buf.Flush(); err != nil {
		if Logger != nil {
			Logger.Warn("Failed to flush output", zap.Error(err))
		}
	}
	if err := w.Close(); err != nil {
		if Logger != nil {
			Logger.Warn("Failed to close writer", zap.Error(err))
		}
	}
}

func composeHandshake(cfg *config.Config, mode string) common.Handshake {
	hs := common.Handshake{Compress: cfg.Compress}
	switch mode {
	case "checksum":
		hs.Checksum = true
	case "checksum-dedup":
		hs.Checksum = true
		hs.ChecksumDedup = true
	}
	return hs
}

func iterateBlocks(cfg *config.Config, ranges []Range, srcFile *os.File, bufOut *bufio.Writer, dedup DeduplicationStrategy, pipeFds [2]int) (int64, int, error) {
	var totalBytes int64
	skippedBlocks := 0
	var header [12]byte
	for _, r := range ranges {
		data, err := ReadBlockWithRetries(cfg, srcFile, r.Start, cfg.ZeroCopy, pipeFds)
		if err != nil {
			return totalBytes, skippedBlocks, fmt.Errorf("error reading block at offset %d: %w", r.Start, err)
		}
		if dedup != nil {
			if !dedup.ShouldTransfer(r.Start, data) {
				skippedBlocks++
				putBlockBuffer(data)
				continue
			}
			dedup.RecordTransfer(r.Start, data)
		}

		binary.BigEndian.PutUint64(header[0:8], uint64(r.Start))
		binary.BigEndian.PutUint32(header[8:12], uint32(cfg.BlockSize))
		if _, err := bufOut.Write(header[:]); err != nil {
			putBlockBuffer(data)
			return totalBytes, skippedBlocks, fmt.Errorf("failed to write header: %w", err)
		}
		if _, err := bufOut.Write(data); err != nil {
			putBlockBuffer(data)
			return totalBytes, skippedBlocks, fmt.Errorf("failed to write block data: %w", err)
		}
		putBlockBuffer(data)

		totalBytes += int64(cfg.BlockSize)
	}
	return totalBytes, skippedBlocks, nil
}

func finalizeProgress(cfg *config.Config) {
	if cfg.Progress {
		fmt.Fprintln(os.Stderr, "")
	}
}

func dumpChangesCore(cfg *config.Config, snapshot, source string, out io.Writer, dedup DeduplicationStrategy, handshake string) error {
	if cfg.BlockSize == 0 {
		bs, err := blocksize.Detect(source)
		if err != nil {
			return fmt.Errorf("auto-detect block size: %w", err)
		}
		cfg.BlockSize = bs
		Logger.Info("Auto-detected block size", zap.Int("blockSize", cfg.BlockSize))
	}
	Logger.Info("Using block size", zap.Int("blockSize", cfg.BlockSize))

	metadataDevice := GetMetadataDevice(snapshot)
	if metadataDevice == "" {
		return fmt.Errorf("failed to determine metadata device from snapshot %s", snapshot)
	}
	blockSize := int64(cfg.BlockSize)
	ranges, err := GetDifferences(metadataDevice, blockSize)
	if err != nil {
		return fmt.Errorf("error getting differences: %w", err)
	}
	Logger.Info("Changed blocks determined", zap.Int("blockCount", len(ranges)))

	if err := common.WriteHandshake(out, composeHandshake(cfg, handshake)); err != nil {
		return err
	}

	compWriter, bufOut, err := prepareOutputWriter(out, cfg)
	if err != nil {
		return err
	}
	defer cleanupOutput(bufOut, compWriter)

	srcFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open source device %s: %w", source, err)
	}
	defer srcFile.Close()

	var pipeFds [2]int
	if cfg.ZeroCopy {
		if err := syscall.Pipe(pipeFds[:]); err != nil {
			return fmt.Errorf("failed to create pipe: %w", err)
		}
		defer syscall.Close(pipeFds[0])
		defer syscall.Close(pipeFds[1])
	} else {
		pipeFds[0], pipeFds[1] = -1, -1
	}

	startTime := time.Now()
	totalBytesTransferred, skippedBlocks, err := iterateBlocks(cfg, ranges, srcFile, bufOut, dedup, pipeFds)
	if err != nil {
		return err
	}
	finalizeProgress(cfg)

	elapsed := time.Since(startTime).Seconds()
	Logger.Info("Sequential transfer complete",
		zap.Int64("bytes", totalBytesTransferred),
		zap.Int("skippedBlocks", skippedBlocks),
		zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytesTransferred)/elapsed/1048576.0))
	return nil
}

func DumpChangesSequential(cfg *config.Config, snapshot, source string, out io.Writer) error {
	dedup := NewDeduplicationStrategy(cfg)
	if dedup != nil {
		defer func() {
			if err := dedup.SaveState(); err != nil {
				Logger.Error("Failed to save dedup state", zap.Error(err))
			}
		}()
	}
	return dumpChangesCore(cfg, snapshot, source, out, dedup, "")
}

func DumpChangesWithDeduplication(cfg *config.Config, snapshot, source string, out io.Writer, dedup DeduplicationStrategy) error {
	return dumpChangesCore(cfg, snapshot, source, out, dedup, "checksum-dedup")
}

func DumpChanges(cfg *config.Config, snapshot, source string, out io.Writer) error {
	dedup := NewDeduplicationStrategy(cfg)
	if dedup != nil {
		defer func() {
			if err := dedup.SaveState(); err != nil {
				Logger.Error("Failed to save dedup state", zap.Error(err))
			}
		}()
		Logger.Info("Deduplication enabled", zap.String("strategy", cfg.DedupStrategy))
		return DumpChangesWithDeduplication(cfg, snapshot, source, out, dedup)
	}
	Logger.Info("Deduplication disabled, performing full block transfer")
	return DumpChangesSequential(cfg, snapshot, source, out)
}

func prepareParallelHandshake(cfg *config.Config) string {
	htokens := []string{common.ProtocolVersion}
	if cfg.VerifyChecksum {
		htokens = append(htokens, "checksum")
	}
	htokens = append(htokens, "compress:"+cfg.Compress)
	return strings.Join(htokens, " ")
}

func processParallelResults(cfg *config.Config, results <-chan *BlockResult, bufOut *bufio.Writer, checksum ChecksumStrategy, totalDataSize int64, startTime time.Time) (int64, error) {
	headerSize := 12
	if cfg.VerifyChecksum {
		headerSize += checksum.Size()
	}
	header := make([]byte, headerSize)
	var totalBytesTransferred int64
	for res := range results {
		if res.Err != nil {
			return totalBytesTransferred, fmt.Errorf("error in block %d: %w", res.Index, res.Err)
		}

		binary.BigEndian.PutUint64(header[0:8], res.Offset)
		binary.BigEndian.PutUint32(header[8:12], res.Size)
		n := 12
		if cfg.VerifyChecksum {
			sum := checksum.Compute(res.Data)
			copy(header[12:], sum)
			n += checksum.Size()
		}

		if _, err := bufOut.Write(header[:n]); err != nil {
			return totalBytesTransferred, fmt.Errorf("failed to write header: %w", err)
		}
		if _, err := bufOut.Write(res.Data); err != nil {
			putBlockBuffer(res.Data)
			return totalBytesTransferred, fmt.Errorf("failed to write data: %w", err)
		}
		putBlockBuffer(res.Data)

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
			Logger.Debug("Parallel dump progress", zap.Int("block", res.Index+1), zap.Float64("MB/s", speed))
		}
	}
	return totalBytesTransferred, nil
}

func worker(cfg *config.Config, srcFile *os.File, tasks <-chan BlockTask, results chan<- *BlockResult) {
	defer workerWG.Done()
	for task := range tasks {
		data, err := ReadBlockWithRetries(cfg, srcFile, task.R.Start, false, [2]int{-1, -1})
		results <- &BlockResult{
			Index:  task.Index,
			Offset: uint64(task.R.Start),
			Size:   uint32(cfg.BlockSize),
			Data:   data,
			Err:    err,
		}
	}
}

func DumpChangesParallel(cfg *config.Config, snapshot, source string, out io.Writer) error {
	if cfg.ZeroCopy {
		Logger.Warn("ZeroCopy mode enabled, falling back to sequential execution")
		return DumpChangesSequential(cfg, snapshot, source, out)
	}

	if cfg.BlockSize == 0 {
		bs, err := blocksize.Detect(source)
		if err != nil {
			return fmt.Errorf("auto-detect block size: %w", err)
		}
		cfg.BlockSize = bs
		Logger.Info("Auto-detected block size", zap.Int("blockSize", cfg.BlockSize))
	}

	metadataDevice := GetMetadataDevice(snapshot)
	if metadataDevice == "" {
		return fmt.Errorf("failed to determine metadata device from snapshot %s", snapshot)
	}
	blockSize := int64(cfg.BlockSize)

	Logger.Info("Using block size", zap.Int("blockSize", cfg.BlockSize))

	ranges, err := GetDifferences(metadataDevice, blockSize)
	if err != nil {
		return fmt.Errorf("error getting differences: %w", err)
	}
	Logger.Info("Changed blocks determined", zap.Int("blockCount", len(ranges)))

	var totalDataSize int64
	for _, r := range ranges {
		totalDataSize += (r.End - r.Start + 1)
	}

	fmt.Fprintln(out, prepareParallelHandshake(cfg))

	compWriter, bufOut, err := prepareOutputWriter(out, cfg)
	if err != nil {
		return err
	}
	defer cleanupOutput(bufOut, compWriter)

	srcFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open source device %s: %w", source, err)
	}
	defer srcFile.Close()

	numBlocks := len(ranges)
	taskBuf := cfg.Parallel
	if taskBuf < 1 {
		taskBuf = 1
	}
	tasks := make(chan BlockTask, taskBuf)
	results := make(chan *BlockResult, taskBuf)
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
	workerWG = &wg
	for i := 0; i < cfg.Parallel; i++ {
		wg.Add(1)
		go worker(cfg, srcFile, tasks, results)
	}

	go func() {
		for i := resumeStart; i < numBlocks; i++ {
			tasks <- BlockTask{Index: i, R: ranges[i]}
		}
		close(tasks)
	}()

	go finalizeResults(&wg, results)

	startTime := time.Now()
	checksum := GetChecksumStrategy(cfg.ChecksumAlgorithm)
	totalBytesTransferred, err := processParallelResults(cfg, results, bufOut, checksum, totalDataSize, startTime)
	if err != nil {
		return err
	}
	finalizeProgress(cfg)

	elapsed := time.Since(startTime).Seconds()
	Logger.Info("Parallel transfer complete",
		zap.Int64("bytes", totalBytesTransferred),
		zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytesTransferred)/elapsed/1048576.0))
	return nil
}

func finalizeResults(wg *sync.WaitGroup, results chan<- *BlockResult) {
	wg.Wait()
	close(results)
}

func readAndValidateHandshake(bufReader *bufio.Reader, dedup DeduplicationStrategy, verify bool) (common.Handshake, error) {
	hs, err := common.ReadHandshake(bufReader)
	if err != nil {
		return common.Handshake{}, fmt.Errorf("failed to read protocol handshake: %w", err)
	}
	if verify && !hs.Checksum {
		return hs, fmt.Errorf("unexpected protocol handshake: %s", hs.String())
	}
	if dedup != nil && !hs.ChecksumDedup {
		return hs, fmt.Errorf("unexpected protocol handshake: %s", hs.String())
	}
	return hs, nil
}

func applyBlocks(cfg *config.Config, reader *bufio.Reader, destFile *os.File, dedup DeduplicationStrategy, verify bool, checksum ChecksumStrategy) (int64, error) {
	var totalBytes int64
	headerLen := 12
	if verify {
		headerLen += checksum.Size()
	}
	headerBuf := make([]byte, headerLen)
	for {
		_, err := io.ReadFull(reader, headerBuf)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return totalBytes, fmt.Errorf("failed to read chunk header: %w", err)
		}

		offset := binary.BigEndian.Uint64(headerBuf[0:8])
		chunkSize := binary.BigEndian.Uint32(headerBuf[8:12])

		var transmittedSum []byte
		if verify {
			transmittedSum = make([]byte, checksum.Size())
			copy(transmittedSum, headerBuf[12:])
		}

		data := getBlockBuffer(int(chunkSize))
		if _, err := io.ReadFull(reader, data); err != nil {
			putBlockBuffer(data)
			return totalBytes, fmt.Errorf("failed to read chunk data: %w", err)
		}

		if verify {
			computed := checksum.Compute(data)
			if !bytes.Equal(transmittedSum, computed) {
				putBlockBuffer(data)
				return totalBytes, fmt.Errorf("checksum mismatch at offset %d", offset)
			}
		}

		if dedup != nil {
			if !dedup.ShouldTransfer(int64(offset), data) {
				putBlockBuffer(data)
				continue
			}
			dedup.RecordTransfer(int64(offset), data)
		}

		if _, err := destFile.Seek(int64(offset), io.SeekStart); err != nil {
			Logger.Warn("Seek error", zap.Uint64("offset", offset), zap.Error(err))
			putBlockBuffer(data)
			continue
		}
		if _, err := destFile.Write(data); err != nil {
			putBlockBuffer(data)
			return totalBytes, fmt.Errorf("failed to write data at offset %d: %w", offset, err)
		}
		putBlockBuffer(data)

		totalBytes += int64(chunkSize)
	}
	return totalBytes, nil
}

func processDumpDataCore(cfg *config.Config, in io.Reader, destPath string, dedup DeduplicationStrategy, verify bool) error {
	bufReader := bufio.NewReader(in)
	hs, err := readAndValidateHandshake(bufReader, dedup, verify)
	if err != nil {
		return err
	}

	decReader, err := NewDecompressionReader(bufReader, hs.Compress, cfg.CompressConcurrency)
	if err != nil {
		return fmt.Errorf("failed to create decompression reader: %w", err)
	}
	defer func() {
		if err := decReader.Close(); err != nil && Logger != nil {
			Logger.Warn("Failed to close decompression reader", zap.Error(err))
		}
	}()

	reader := bufio.NewReader(decReader)

	destFile, err := os.OpenFile(destPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open destination device %s: %w", destPath, err)
	}
	defer destFile.Close()

	startTime := time.Now()
	checksum := GetChecksumStrategy(cfg.ChecksumAlgorithm)
	totalBytes, err := applyBlocks(cfg, reader, destFile, dedup, verify, checksum)
	if err != nil {
		return err
	}

	elapsed := time.Since(startTime).Seconds()
	Logger.Info("Applied changes",
		zap.Int64("bytes", totalBytes),
		zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(totalBytes)/elapsed/1048576.0))
	return nil
}

func ProcessDumpDataWithDeduplication(cfg *config.Config, in io.Reader, destPath string, dedup DeduplicationStrategy) error {
	return processDumpDataCore(cfg, in, destPath, dedup, false)
}

func ProcessDumpData(cfg *config.Config, in io.Reader, destPath string) error {
	return processDumpDataCore(cfg, in, destPath, nil, true)
}

func RunApply(cfg *config.Config, applyFile, destDevice string) error {
	var in io.Reader
	if applyFile == "-" {
		in = os.Stdin
	} else {
		f, err := os.Open(applyFile)
		if err != nil {
			return fmt.Errorf("failed to open apply file %s: %w", applyFile, err)
		}
		defer f.Close()
		in = f
	}

	dedup := NewDeduplicationStrategy(cfg)
	if dedup != nil {
		Logger.Info("Applying deduplication during restore", zap.String("strategy", cfg.DedupStrategy))
		defer func() {
			if err := dedup.SaveState(); err != nil {
				Logger.Error("Failed to save dedup state", zap.Error(err))
			}
		}()
		return ProcessDumpDataWithDeduplication(cfg, in, destDevice, dedup)
	}

	return ProcessDumpData(cfg, in, destDevice)
}
