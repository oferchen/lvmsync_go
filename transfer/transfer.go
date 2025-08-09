// transfer/transfer.go
package transfer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"math"
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

// Logger holds the package-wide zap.Logger used for progress and error reporting.
var Logger *zap.Logger
var workerWG *sync.WaitGroup

// SetLogger assigns Logger for package-wide logging and overrides any existing logger.
func SetLogger(logger *zap.Logger) {
	Logger = logger
}

// ChecksumState stores block checksums and the algorithm used for deduplication.
type ChecksumState struct {
	Checksums map[uint64][]byte
	Strategy  string
}

func LoadChecksumState(filename string) (state *ChecksumState, err error) {
	var file *os.File
	file, err = os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return &ChecksumState{Checksums: make(map[uint64][]byte), Strategy: "sha256"}, nil
		}
		return nil, fmt.Errorf("open checksum state: %w", err)
	}
	defer common.CloseWithErr(file, &err, "close checksum state file")

	state = &ChecksumState{}
	decoder := gob.NewDecoder(file)
	if err = decoder.Decode(state); err != nil {
		return nil, fmt.Errorf("decode checksum state: %w", err)
	}

	if state.Checksums == nil {
		state.Checksums = make(map[uint64][]byte)
	}
	if state.Strategy == "" {
		state.Strategy = "sha256"
	}
	return state, nil
}

//revive:disable-next-line:cognitive-complexity
func SaveChecksumState(filename string, state *ChecksumState) (err error) {
	var file *os.File
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("create checksum state: %w", err)
	}
	if err = file.Chmod(0o600); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			if Logger != nil {
				Logger.Warn("Failed to close checksum state file", zap.Error(closeErr))
			}
			return fmt.Errorf("chmod checksum state: %v; close checksum state: %w", err, closeErr)
		}
		return fmt.Errorf("chmod checksum state: %w", err)
	}
	defer common.CloseWithErr(file, &err, "close checksum state file")

	encoder := gob.NewEncoder(file)
	if err = encoder.Encode(state); err != nil {
		return fmt.Errorf("encode checksum state: %w", err)
	}
	return nil
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
	case StrategyChecksum:
		hs.Checksum = true
	case StrategyChecksum + "-dedup":
		hs.Checksum = true
		hs.ChecksumDedup = true
	}
	return hs
}

func validateOffsetAndSize(offset uint64, size int) error {
	if offset > math.MaxInt64 {
		return fmt.Errorf("offset %d overflows int64", offset)
	}
	if size < 0 || size > int(math.MaxUint32) {
		return fmt.Errorf("invalid block size %d: must be between 0 and %d", size, uint32(math.MaxUint32))
	}
	return nil
}

func iterateBlocks(cfg *config.Config, ranges []Range, srcFile *os.File, bufOut *bufio.Writer, dedup DeduplicationStrategy, pipeFds [2]int) (int64, int, error) {
	var totalBytes int64
	skippedBlocks := 0
	var header [12]byte
	for _, r := range ranges {
		if err := validateOffsetAndSize(r.Start, cfg.BlockSize); err != nil {
			return totalBytes, skippedBlocks, err
		}
		data, err := ReadBlockWithRetries(cfg, srcFile, int64(r.Start), cfg.ZeroCopy, pipeFds)
		if err != nil {
			return totalBytes, skippedBlocks, fmt.Errorf("error reading block at offset %d: %w", r.Start, err)
		}
		if dedup != nil {
			if !dedup.ShouldTransfer(int64(r.Start), data) {
				skippedBlocks++
				putBlockBuffer(data)
				continue
			}
			dedup.RecordTransfer(int64(r.Start), data)
		}

		binary.BigEndian.PutUint64(header[0:8], r.Start)
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

func detectBlockSize(cfg *config.Config, source string) error {
	if cfg.BlockSize != 0 {
		return nil
	}
	bs, err := blocksize.Detect(source)
	if err != nil {
		return fmt.Errorf("auto-detect block size: %w", err)
	}
	cfg.BlockSize = bs
	Logger.Info("Auto-detected block size", zap.Int("blockSize", cfg.BlockSize))
	return nil
}

func gatherChangedRanges(snapshot string, blockSize int64) ([]Range, error) {
	metadataDevice := GetMetadataDevice(snapshot)
	if metadataDevice == "" {
		return nil, fmt.Errorf("failed to determine metadata device from snapshot %s", snapshot)
	}
	ranges, err := GetDifferences(metadataDevice, blockSize)
	if err != nil {
		return nil, fmt.Errorf("error getting differences: %w", err)
	}
	Logger.Info("Changed blocks determined", zap.Int("blockCount", len(ranges)))
	return ranges, nil
}

func setupOutput(cfg *config.Config, out io.Writer, handshake string) (io.WriteCloser, *bufio.Writer, error) {
	if err := common.WriteHandshake(out, composeHandshake(cfg, handshake)); err != nil {
		return nil, nil, err
	}
	return prepareOutputWriter(out, cfg)
}

func setupSourceFile(source string) (*os.File, error) {
	srcFile, err := os.Open(source)
	if err != nil {
		return nil, fmt.Errorf("failed to open source device %s: %w", source, err)
	}
	return srcFile, nil
}

func setupPipe(cfg *config.Config) ([2]int, func(), error) {
	var pipeFds [2]int
	cleanup := func() {}
	if cfg.ZeroCopy {
		if err := syscall.Pipe(pipeFds[:]); err != nil {
			return pipeFds, cleanup, fmt.Errorf("failed to create pipe: %w", err)
		}
		cleanup = func() {
			if closeErr := syscall.Close(pipeFds[0]); closeErr != nil {
				Logger.Warn("close pipe", zap.Int("fd", pipeFds[0]), zap.Error(closeErr))
			}
			if closeErr := syscall.Close(pipeFds[1]); closeErr != nil {
				Logger.Warn("close pipe", zap.Int("fd", pipeFds[1]), zap.Error(closeErr))
			}
		}
	} else {
		pipeFds[0], pipeFds[1] = -1, -1
	}
	return pipeFds, cleanup, nil
}

func logSequentialSummary(bytes int64, skipped int, start time.Time) {
	elapsed := time.Since(start).Seconds()
	Logger.Info("Sequential transfer complete",
		zap.Int64("bytes", bytes),
		zap.Int("skippedBlocks", skipped),
		zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(bytes)/elapsed/1048576.0))
}

func dumpChangesCore(cfg *config.Config, snapshot, source string, out io.Writer, dedup DeduplicationStrategy, handshake string) (err error) {
	if err = detectBlockSize(cfg, source); err != nil {
		return err
	}
	Logger.Info("Using block size", zap.Int("blockSize", cfg.BlockSize))

	ranges, err := gatherChangedRanges(snapshot, int64(cfg.BlockSize))
	if err != nil {
		return err
	}

	compWriter, bufOut, err := setupOutput(cfg, out, handshake)
	if err != nil {
		return err
	}
	defer cleanupOutput(bufOut, compWriter)

	srcFile, err := setupSourceFile(source)
	if err != nil {
		return err
	}
	defer common.CloseWithErr(srcFile, &err, "close source file")

	pipeFds, cleanupPipe, err := setupPipe(cfg)
	if err != nil {
		return err
	}
	defer cleanupPipe()

	startTime := time.Now()
	var totalBytesTransferred int64
	var skippedBlocks int
	totalBytesTransferred, skippedBlocks, err = iterateBlocks(cfg, ranges, srcFile, bufOut, dedup, pipeFds)
	if err != nil {
		return err
	}
	finalizeProgress(cfg)

	logSequentialSummary(totalBytesTransferred, skippedBlocks, startTime)
	return nil
}

// DumpChangesSequential streams changed blocks from snapshot to out sequentially and saves dedup state if enabled.
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

// DumpChangesWithDeduplication transfers changed blocks using the provided dedup strategy and a checksum-dedup handshake, updating the strategy's state.
func DumpChangesWithDeduplication(cfg *config.Config, snapshot, source string, out io.Writer, dedup DeduplicationStrategy) error {
	return dumpChangesCore(cfg, snapshot, source, out, dedup, "checksum-dedup")
}

// DumpChanges chooses an appropriate transfer mode and persists dedup state when a strategy is configured.
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
		htokens = append(htokens, StrategyChecksum)
	}
	htokens = append(htokens, "compress:"+cfg.Compress)
	return strings.Join(htokens, " ")
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

func writeResult(bufOut *bufio.Writer, header []byte, data []byte) error {
	if _, err := bufOut.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := bufOut.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}
	return nil
}

func saveResumeState(cfg *config.Config, index int) {
	if cfg.ResumeState == "" {
		return
	}
	err := os.WriteFile(cfg.ResumeState, []byte(fmt.Sprintf("%d", index+1)), 0o600)
	if err != nil {
		Logger.Warn("Failed to update resume state", zap.Error(err))
	}
}

func reportProgress(cfg *config.Config, transferred, total int64, index int, start time.Time) {
	if cfg.Progress {
		progressPercent := float64(transferred) / float64(total) * 100.0
		fmt.Fprintf(os.Stderr, "\rProgress: %.2f%%", progressPercent)
	}
	if cfg.Verbose > 0 && index > 0 && index%100 == 0 {
		elapsed := time.Since(start).Seconds()
		speed := float64(transferred) / elapsed / 1048576.0
		Logger.Debug("Parallel dump progress", zap.Int("block", index+1), zap.Float64("MB/s", speed))
	}
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

		n := prepareResultHeader(cfg, checksum, res, header)
		if err := writeResult(bufOut, header[:n], res.Data); err != nil {
			putBlockBuffer(res.Data)
			return totalBytesTransferred, err
		}
		putBlockBuffer(res.Data)

		totalBytesTransferred += int64(res.Size)
		saveResumeState(cfg, res.Index)
		reportProgress(cfg, totalBytesTransferred, totalDataSize, res.Index, startTime)
	}
	return totalBytesTransferred, nil
}

func worker(cfg *config.Config, srcFile *os.File, tasks <-chan BlockTask, results chan<- *BlockResult) {
	defer workerWG.Done()
	for task := range tasks {
		if err := validateOffsetAndSize(task.R.Start, cfg.BlockSize); err != nil {
			results <- &BlockResult{Index: task.Index, Err: err}
			continue
		}
		data, err := ReadBlockWithRetries(cfg, srcFile, int64(task.R.Start), false, [2]int{-1, -1})
		results <- &BlockResult{
			Index:  task.Index,
			Offset: task.R.Start,
			Size:   uint32(cfg.BlockSize),
			Data:   data,
			Err:    err,
		}
	}
}

// DumpChangesParallel transfers changed blocks using multiple goroutines and updates resume state as blocks complete.

func calculateTotalDataSize(ranges []Range) int64 {
	var total uint64
	for _, r := range ranges {
		if r.End < r.Start {
			continue
		}
		total += r.End - r.Start + 1
	}
	if total > uint64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(total)
}

func writeParallelHandshake(cfg *config.Config, out io.Writer) error {
	_, err := fmt.Fprintln(out, prepareParallelHandshake(cfg))
	return err
}

func readResumeStart(cfg *config.Config) int {
	if cfg.ResumeState == "" {
		return 0
	}
	data, err := os.ReadFile(cfg.ResumeState)
	if err != nil {
		return 0
	}
	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	Logger.Info("Resuming from block", zap.Int("resumeStart", val))
	return val
}

func startParallelWorkers(cfg *config.Config, srcFile *os.File, ranges []Range, resumeStart int) <-chan *BlockResult {
	numBlocks := len(ranges)
	taskBuf := cfg.Parallel
	if taskBuf < 1 {
		taskBuf = 1
	}
	tasks := make(chan BlockTask, taskBuf)
	results := make(chan *BlockResult, taskBuf)
	workerWG = &sync.WaitGroup{}
	for i := 0; i < cfg.Parallel; i++ {
		workerWG.Add(1)
		go worker(cfg, srcFile, tasks, results)
	}

	go func() {
		for i := resumeStart; i < numBlocks; i++ {
			tasks <- BlockTask{Index: i, R: ranges[i]}
		}
		close(tasks)
	}()

	go finalizeResults(workerWG, results)
	return results
}

func logParallelSummary(bytes int64, start time.Time) {
	elapsed := time.Since(start).Seconds()
	Logger.Info("Parallel transfer complete",
		zap.Int64("bytes", bytes),
		zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(bytes)/elapsed/1048576.0))
}

func DumpChangesParallel(cfg *config.Config, snapshot, source string, out io.Writer) (err error) {
	if cfg.ZeroCopy {
		Logger.Warn("ZeroCopy mode enabled, falling back to sequential execution")
		return DumpChangesSequential(cfg, snapshot, source, out)
	}

	if err = detectBlockSize(cfg, source); err != nil {
		return err
	}

	Logger.Info("Using block size", zap.Int("blockSize", cfg.BlockSize))
	ranges, err := gatherChangedRanges(snapshot, int64(cfg.BlockSize))
	if err != nil {
		return err
	}

	totalDataSize := calculateTotalDataSize(ranges)
	if err = writeParallelHandshake(cfg, out); err != nil {
		return err
	}

	compWriter, bufOut, err := prepareOutputWriter(out, cfg)
	if err != nil {
		return err
	}
	defer cleanupOutput(bufOut, compWriter)

	srcFile, err := setupSourceFile(source)
	if err != nil {
		return err
	}
	defer common.CloseWithErr(srcFile, &err, "close source file")

	resumeStart := readResumeStart(cfg)
	results := startParallelWorkers(cfg, srcFile, ranges, resumeStart)

	startTime := time.Now()
	checksum := GetChecksumStrategy(cfg.ChecksumAlgorithm)
	var totalBytesTransferred int64
	totalBytesTransferred, err = processParallelResults(cfg, results, bufOut, checksum, totalDataSize, startTime)
	if err != nil {
		return err
	}
	finalizeProgress(cfg)
	logParallelSummary(totalBytesTransferred, startTime)
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

func readBlockData(reader io.Reader, chunkSize uint32) ([]byte, error) {
	data := getBlockBuffer(int(chunkSize))
	if _, err := io.ReadFull(reader, data); err != nil {
		putBlockBuffer(data)
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

func writeData(destFile *os.File, offset uint64, data []byte) error {
	if offset > math.MaxInt64 {
		return fmt.Errorf("offset %d overflows int64", offset)
	}
	if _, err := destFile.Seek(int64(offset), io.SeekStart); err != nil {
		Logger.Warn("Seek error", zap.Uint64("offset", offset), zap.Error(err))
		return nil
	}
	if _, err := destFile.Write(data); err != nil {
		return fmt.Errorf("failed to write data at offset %d: %w", offset, err)
	}
	return nil
}

func processBlock(destFile *os.File, dedup DeduplicationStrategy, verify bool, checksum ChecksumStrategy, offset uint64, transmitted []byte, data []byte) (bool, error) {
	if err := verifyChecksum(verify, checksum, data, transmitted, offset); err != nil {
		return false, err
	}
	if dedup != nil {
		if !dedup.ShouldTransfer(int64(offset), data) {
			return false, nil
		}
		dedup.RecordTransfer(int64(offset), data)
	}
	if err := writeData(destFile, offset, data); err != nil {
		return false, err
	}
	return true, nil
}

func applyBlocks(reader *bufio.Reader, destFile *os.File, dedup DeduplicationStrategy, verify bool, checksum ChecksumStrategy) (int64, error) {
	var totalBytes int64
	headerLen := 12
	if verify {
		headerLen += checksum.Size()
	}
	headerBuf := make([]byte, headerLen)
	for {
		offset, chunkSize, transmittedSum, err := readBlockHeader(reader, headerBuf, verify, checksum)
		if err == io.EOF {
			break
		}
		if err != nil {
			return totalBytes, err
		}

		data, err := readBlockData(reader, chunkSize)
		if err == io.EOF {
			break
		}
		if err != nil {
			return totalBytes, err
		}
		written, err := processBlock(destFile, dedup, verify, checksum, offset, transmittedSum, data)
		putBlockBuffer(data)
		if err != nil {
			return totalBytes, err
		}
		if written {
			totalBytes += int64(chunkSize)
		}
	}
	return totalBytes, nil
}

func processDumpDataCore(cfg *config.Config, in io.Reader, destPath string, dedup DeduplicationStrategy, verify bool) (err error) {
	bufReader := bufio.NewReader(in)
	var hs common.Handshake
	hs, err = readAndValidateHandshake(bufReader, dedup, verify)
	if err != nil {
		return err
	}

	decReader, err := NewDecompressionReader(bufReader, hs.Compress, cfg.CompressConcurrency)
	if err != nil {
		return fmt.Errorf("failed to create decompression reader: %w", err)
	}
	defer func() {
		if closeErr := decReader.Close(); closeErr != nil && Logger != nil {
			Logger.Warn("Failed to close decompression reader", zap.Error(closeErr))
		}
	}()

	reader := bufio.NewReader(decReader)

	destFile, err := os.OpenFile(destPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open destination device %s: %w", destPath, err)
	}
	defer common.CloseWithErr(destFile, &err, "close destination device")

	startTime := time.Now()
	checksum := GetChecksumStrategy(cfg.ChecksumAlgorithm)
	var totalBytes int64
	totalBytes, err = applyBlocks(reader, destFile, dedup, verify, checksum)
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

// ProcessDumpDataWithDeduplication applies a dump stream to destPath using the given dedup strategy without checksum verification, updating the strategy's state.
func ProcessDumpDataWithDeduplication(cfg *config.Config, in io.Reader, destPath string, dedup DeduplicationStrategy) error {
	return processDumpDataCore(cfg, in, destPath, dedup, false)
}

// ProcessDumpData applies a dump stream to destPath with checksum verification for each block before writing.
func ProcessDumpData(cfg *config.Config, in io.Reader, destPath string) error {
	return processDumpDataCore(cfg, in, destPath, nil, true)
}

func openApplyReader(applyFile string) (io.ReadCloser, error) {
	if applyFile == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	f, err := os.Open(applyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open apply file %s: %w", applyFile, err)
	}
	return f, nil
}

func applyData(cfg *config.Config, in io.Reader, destDevice string) error {
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

// RunApply reads a dump file or stdin and writes the data to destDevice.
func RunApply(cfg *config.Config, applyFile, destDevice string) (err error) {
	rc, err := openApplyReader(applyFile)
	if err != nil {
		return err
	}
	defer common.CloseWithErr(rc, &err, "close apply file")
	return applyData(cfg, rc, destDevice)
}
