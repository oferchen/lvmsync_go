// transfer/transfer.go
package transfer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/config"
	"lvmsync_go/internal/blocksize"
)

// Transfer encapsulates transfer state shared across operations.
type Transfer struct {
	Logger   *zap.Logger
	workerWG *sync.WaitGroup
}

// NewTransfer creates a Transfer with the provided logger and wait group.
// When wg is nil, a new instance is allocated.
func NewTransfer(logger *zap.Logger, wg *sync.WaitGroup) *Transfer {
	if wg == nil {
		wg = &sync.WaitGroup{}
	}
	return &Transfer{Logger: logger, workerWG: wg}
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
func SaveChecksumState(filename string, state *ChecksumState, logger *zap.Logger) (err error) {
	var file *os.File
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("create checksum state: %w", err)
	}
	if err = file.Chmod(0o600); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			if logger != nil {
				logger.Warn("Failed to close checksum state file", zap.Error(closeErr))
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

func cleanupOutput(buf *bufio.Writer, w io.WriteCloser, logger *zap.Logger) {
	if err := buf.Flush(); err != nil {
		if logger != nil {
			logger.Warn("Failed to flush output", zap.Error(err))
		}
	}
	if err := w.Close(); err != nil {
		if logger != nil {
			logger.Warn("Failed to close writer", zap.Error(err))
		}
	}
}

func detectBlockSize(cfg *config.Config, source string, logger *zap.Logger) error {
	if cfg.BlockSize != 0 {
		return nil
	}
	bs, err := blocksize.Detect(source)
	if err != nil {
		return fmt.Errorf("auto-detect block size: %w", err)
	}
	cfg.BlockSize = bs
	if logger != nil {
		logger.Info("Auto-detected block size", zap.Int("block_size_bytes", cfg.BlockSize))
	}
	return nil
}

func gatherChangedRanges(snapshot string, blockSize int64, logger *zap.Logger) ([]Range, error) {
	metadataDevice := GetMetadataDevice(snapshot)
	if metadataDevice == "" {
		return nil, fmt.Errorf("failed to determine metadata device from snapshot %s", snapshot)
	}
	ranges, err := GetDifferences(metadataDevice, blockSize)
	if err != nil {
		return nil, fmt.Errorf("error getting differences: %w", err)
	}
	if logger != nil {
		logger.Info("Changed blocks determined", zap.Int("block_count", len(ranges)))
	}
	return ranges, nil
}

func prepareRanges(cfg *config.Config, snapshot, source string, logger *zap.Logger) ([]Range, error) {
	if err := detectBlockSize(cfg, source, logger); err != nil {
		return nil, err
	}
	if logger != nil {
		logger.Info("Using block size", zap.Int("block_size_bytes", cfg.BlockSize))
	}

	_, blockSize, err := validateOffsetAndSize(0, cfg.BlockSize)
	if err != nil {
		return nil, err
	}
	ranges, err := gatherChangedRanges(snapshot, int64(blockSize), logger)
	if err != nil {
		return nil, err
	}
	return ranges, nil
}

func setupSourceFile(cfg *config.Config, source string) (*os.File, error) {
	if cfg.ODirect {
		tmp, err := os.Open(source)
		if err != nil {
			return nil, fmt.Errorf("failed to open source device %s: %w", source, err)
		}
		sector, err := DetectSectorSize(tmp)
		_ = tmp.Close()
		if err == nil && cfg.BlockSize%sector == 0 {
			if f, direct, err := openFileODirect(source, os.O_RDONLY); err == nil && direct {
				return f, nil
			}
		}
	}
	srcFile, err := os.Open(source)
	if err != nil {
		return nil, fmt.Errorf("failed to open source device %s: %w", source, err)
	}
	return srcFile, nil
}

func setupPipe(cfg *config.Config, logger *zap.Logger) ([2]int, func(), error) {
	var pipeFds [2]int
	cleanup := func() {}
	if cfg.ZeroCopy {
		if err := syscall.Pipe(pipeFds[:]); err != nil {
			return pipeFds, cleanup, fmt.Errorf("failed to create pipe: %w", err)
		}
		cleanup = func() {
			if closeErr := syscall.Close(pipeFds[0]); closeErr != nil {
				if logger != nil {
					logger.Warn("close pipe", zap.Int("fd", pipeFds[0]), zap.Error(closeErr))
				}
			}
			if closeErr := syscall.Close(pipeFds[1]); closeErr != nil {
				if logger != nil {
					logger.Warn("close pipe", zap.Int("fd", pipeFds[1]), zap.Error(closeErr))
				}
			}
		}
	} else {
		pipeFds[0], pipeFds[1] = -1, -1
	}
	return pipeFds, cleanup, nil
}

func (t *Transfer) dumpChangesCore(cfg *config.Config, snapshot, source string, out io.Writer, dedup DeduplicationStrategy, handshake string) (err error) {
	ranges, err := prepareRanges(cfg, snapshot, source, t.Logger)
	if err != nil {
		return err
	}

	compWriter, bufOut, err := setupOutput(cfg, out, handshake, t.Logger)
	if err != nil {
		return err
	}
	defer cleanupOutput(bufOut, compWriter, t.Logger)

	srcFile, err := setupSourceFile(cfg, source)
	if err != nil {
		return err
	}
	defer common.CloseWithErr(srcFile, &err, "close source file")

	pipeFds, cleanupPipe, err := setupPipe(cfg, t.Logger)
	if err != nil {
		return err
	}
	defer cleanupPipe()

	resumeDigest := readResumeDigest(cfg, t.Logger)
	startIdx := findResumeIndex(cfg, srcFile, ranges, resumeDigest, t.Logger)
	if startIdx > 0 {
		ranges = ranges[startIdx:]
	}

	startTime := time.Now()
	var totalBytesTransferred int64
	var skippedBlocks int
	var manifest *Manifest
	totalBytesTransferred, skippedBlocks, manifest, err = iterateBlocks(cfg, ranges, srcFile, bufOut, dedup, pipeFds, t.Logger)
	if err != nil {
		return err
	}
	finalizeProgress(cfg, t.Logger)

	logSequentialSummary(t.Logger, totalBytesTransferred, skippedBlocks, startTime)
	finalizeResumeState(cfg, t.Logger)
	if manifest != nil {
		if t.Logger != nil {
			t.Logger.Info("final checksum", zap.String("final_digest", fmt.Sprintf("%x", manifest.FinalDigest)))
		}
	}
	if t.Logger != nil {
		_ = t.Logger.Sync()
	}
	return nil
}

func (t *Transfer) setupDedup(cfg *config.Config) (DeduplicationStrategy, func()) {
	dedup := NewDeduplicationStrategy(cfg, t.Logger)
	cleanup := func() {}
	if dedup != nil {
		cleanup = func() {
			if err := dedup.SaveState(); err != nil {
				if t.Logger != nil {
					t.Logger.Error("Failed to save dedup state", zap.Error(err))
				}
			}
		}
	}
	return dedup, cleanup
}

// DumpChangesSequential streams changed blocks from snapshot to out sequentially and saves dedup state if enabled.
func (t *Transfer) DumpChangesSequential(cfg *config.Config, snapshot, source string, out io.Writer) error {
	dedup, cleanup := t.setupDedup(cfg)
	if dedup != nil {
		defer cleanup()
	}
	return t.dumpChangesCore(cfg, snapshot, source, out, dedup, "")
}

// DumpChangesWithDeduplication transfers changed blocks using the provided dedup strategy and a checksum-dedup handshake, updating the strategy's state.
func (t *Transfer) DumpChangesWithDeduplication(cfg *config.Config, snapshot, source string, out io.Writer, dedup DeduplicationStrategy) error {
	return t.dumpChangesCore(cfg, snapshot, source, out, dedup, "checksum-dedup")
}

// DumpChanges chooses an appropriate transfer mode and persists dedup state when a strategy is configured.
func (t *Transfer) DumpChanges(cfg *config.Config, snapshot, source string, out io.Writer) error {
	dedup, cleanup := t.setupDedup(cfg)
	if dedup != nil {
		defer cleanup()
		if t.Logger != nil {
			t.Logger.Info("Deduplication enabled", zap.String("strategy", cfg.DedupStrategy))
		}
		return t.DumpChangesWithDeduplication(cfg, snapshot, source, out, dedup)
	}
	if t.Logger != nil {
		t.Logger.Info("Deduplication disabled, performing full block transfer")
	}
	return t.DumpChangesSequential(cfg, snapshot, source, out)
}

var (
	resumeMu    sync.Mutex
	resumeBytes int64
	resumeLast  time.Time
	resumeChunk [32]byte
)

func writeResumeState(logger *zap.Logger, path string, chunk [32]byte) {
	err := os.WriteFile(path, []byte(hex.EncodeToString(chunk[:])), 0o600)
	if err != nil {
		if logger != nil {
			logger.Warn("Failed to update resume state", zap.Error(err))
		}
	}
}

func saveResumeState(cfg *config.Config, chunk [32]byte, size int64, logger *zap.Logger) {
	if cfg.ResumeState == "" {
		return
	}
	resumeMu.Lock()
	defer resumeMu.Unlock()
	if resumeLast.IsZero() {
		resumeLast = time.Now()
	}
	resumeBytes += size
	resumeChunk = chunk
	if resumeBytes >= 1<<30 || time.Since(resumeLast) >= 10*time.Second {
		writeResumeState(logger, cfg.ResumeState, resumeChunk)
		resumeBytes = 0
		resumeLast = time.Now()
	}
}

func finalizeResumeState(cfg *config.Config, logger *zap.Logger) {
	if cfg.ResumeState == "" {
		return
	}
	resumeMu.Lock()
	defer resumeMu.Unlock()
	if resumeLast.IsZero() {
		return
	}
	writeResumeState(logger, cfg.ResumeState, resumeChunk)
	resumeBytes = 0
}

func readResumeDigest(cfg *config.Config, logger *zap.Logger) [32]byte {
	var out [32]byte
	if cfg.ResumeState == "" {
		return out
	}
	data, err := os.ReadFile(cfg.ResumeState)
	if err != nil {
		return out
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(b) != 32 {
		return out
	}
	copy(out[:], b)
	if logger != nil {
		logger.Info("Resuming from chunk", zap.String("resume_chunk", hex.EncodeToString(out[:])))
	}
	return out
}

func findResumeIndex(cfg *config.Config, srcFile *os.File, ranges []Range, digest [32]byte, logger *zap.Logger) int {
	if cfg.ResumeState == "" || digest == [32]byte{} {
		return 0
	}
	for i, r := range ranges {
		offset, _, err := validateOffsetAndSize(r.Start, cfg.BlockSize)
		if err != nil {
			return 0
		}
		data, err := ReadBlockWithRetries(cfg, srcFile, offset, cfg.ZeroCopy, [2]int{-1, -1}, logger)
		if err != nil {
			continue
		}
		sum := blake3.Sum256(data)
		putBlockBuffer(data)
		if sum == digest {
			if logger != nil {
				logger.Info("Resuming after index", zap.Int("resume_index", i+1))
			}
			return i + 1
		}
	}
	return 0
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

func (t *Transfer) startParallelWorkers(cfg *config.Config, srcFile *os.File, ranges []Range, resumeStart int, logger *zap.Logger) <-chan *BlockResult {
	numBlocks := len(ranges)
	taskBuf := cfg.Parallel
	if taskBuf < 1 {
		taskBuf = 1
	}
	tasks := make(chan BlockTask, taskBuf)
	results := make(chan *BlockResult, taskBuf)
	for i := 0; i < cfg.Parallel; i++ {
		t.workerWG.Add(1)
		go worker(cfg, srcFile, tasks, results, t.workerWG, logger)
	}

	go func() {
		for i := resumeStart; i < numBlocks; i++ {
			tasks <- BlockTask{Index: i, R: ranges[i]}
		}
		close(tasks)
	}()

	go finalizeResults(t.workerWG, results)
	return results
}

func (t *Transfer) DumpChangesParallel(cfg *config.Config, snapshot, source string, out io.Writer) (err error) {
	if cfg.ZeroCopy {
		if t.Logger != nil {
			t.Logger.Warn("ZeroCopy mode enabled, falling back to sequential execution")
		}
		return t.DumpChangesSequential(cfg, snapshot, source, out)
	}

	ranges, err := prepareRanges(cfg, snapshot, source, t.Logger)
	if err != nil {
		return err
	}

	totalDataSize := calculateTotalDataSize(ranges)
	if err = writeParallelHandshake(cfg, out); err != nil {
		return err
	}

	compWriter, bufOut, err := prepareOutputWriter(out, cfg, t.Logger)
	if err != nil {
		return err
	}
	defer cleanupOutput(bufOut, compWriter, t.Logger)

	srcFile, err := setupSourceFile(cfg, source)
	if err != nil {
		return err
	}
	defer common.CloseWithErr(srcFile, &err, "close source file")

	resumeDigest := readResumeDigest(cfg, t.Logger)
	resumeStart := findResumeIndex(cfg, srcFile, ranges, resumeDigest, t.Logger)
	results := t.startParallelWorkers(cfg, srcFile, ranges, resumeStart, t.Logger)

	startTime := time.Now()
	checksum := GetChecksumStrategy(cfg.ChecksumAlgorithm)
	var totalBytesTransferred int64
	var manifest *Manifest
	totalBytesTransferred, manifest, err = processParallelResults(cfg, results, bufOut, checksum, totalDataSize, startTime, t.Logger)
	if err != nil {
		return err
	}
	finalizeProgress(cfg, t.Logger)
	logParallelSummary(t.Logger, totalBytesTransferred, startTime)
	finalizeResumeState(cfg, t.Logger)
	if manifest != nil && t.Logger != nil {
		t.Logger.Info("final checksum", zap.String("final_digest", fmt.Sprintf("%x", manifest.FinalDigest)))
	}
	if t.Logger != nil {
		_ = t.Logger.Sync()
	}
	return nil
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

func writeData(destFile *os.File, offset uint64, data []byte, logger *zap.Logger) error {
	if offset > math.MaxInt64 {
		return fmt.Errorf("offset %d overflows int64", offset)
	}
	if _, err := destFile.Seek(int64(offset), io.SeekStart); err != nil {
		if logger != nil {
			logger.Warn("Seek error", zap.Uint64("offset", offset), zap.Error(err))
		}
		return nil
	}
	if _, err := destFile.Write(data); err != nil {
		return fmt.Errorf("failed to write data at offset %d: %w", offset, err)
	}
	return nil
}

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

func applyBlocks(cfg *config.Config, reader *bufio.Reader, destFile *os.File, dedup DeduplicationStrategy, verify bool, checksum ChecksumStrategy, logger *zap.Logger) (int64, error) {
	var totalBytes int64
	var sinceSync int64
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

		data, err := readBlockData(cfg, reader, chunkSize)
		if err == io.EOF {
			break
		}
		if err != nil {
			return totalBytes, err
		}
		written, err := processBlock(cfg, destFile, dedup, verify, checksum, offset, transmittedSum, data, chunkSize, logger)
		if cfg.ODirect {
			if data != nil {
				putAlignedBlockBuffer(data)
			}
		} else if data != nil {
			putBlockBuffer(data)
		}
		if err != nil {
			return totalBytes, err
		}
		if written {
			totalBytes += int64(chunkSize)
			sinceSync += int64(chunkSize)
			if cfg.SyncIntervalBytes > 0 && sinceSync >= int64(cfg.SyncIntervalBytes) {
				if err := fdatasyncFile(destFile); err != nil {
					return totalBytes, err
				}
				sinceSync = 0
			}
		}
	}
	return totalBytes, nil
}

func (t *Transfer) processDumpDataCore(cfg *config.Config, in io.Reader, destPath string, dedup DeduplicationStrategy, verify bool) (err error) {
	bufReader := bufio.NewReader(in)
	var hs common.Handshake
	hs, err = readAndValidateHandshake(bufReader, cfg, dedup, verify)
	if err != nil {
		return err
	}

	decReader, err := NewDecompressionReader(bufReader, hs.Compress, cfg.CompressConcurrency)
	if err != nil {
		return fmt.Errorf("failed to create decompression reader: %w", err)
	}
	defer func() {
		if closeErr := decReader.Close(); closeErr != nil && t.Logger != nil {
			t.Logger.Warn("Failed to close decompression reader", zap.Error(closeErr))
		}
	}()

	reader := bufio.NewReader(decReader)

	var destFile *os.File
	if cfg.ODirect {
		tmp, err2 := os.Open(destPath)
		if err2 != nil {
			return fmt.Errorf("failed to open destination device %s: %w", destPath, err2)
		}
		sector, err2 := DetectSectorSize(tmp)
		_ = tmp.Close()
		if err2 == nil && cfg.BlockSize%sector == 0 {
			if f, direct, err2 := openFileODirect(destPath, os.O_RDWR); err2 == nil && direct {
				destFile = f
			}
		}
	}
	if destFile == nil {
		destFile, err = os.OpenFile(destPath, os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("failed to open destination device %s: %w", destPath, err)
		}
	}
	defer common.CloseWithErr(destFile, &err, "close destination device")

	startTime := time.Now()
	checksum := GetChecksumStrategy(cfg.ChecksumAlgorithm)
	var totalBytes int64
	totalBytes, err = applyBlocks(cfg, reader, destFile, dedup, verify, checksum, t.Logger)
	if err != nil {
		return err
	}

	elapsed := time.Since(startTime).Seconds()
	if t.Logger != nil {
		t.Logger.Info("Applied changes",
			zap.Int64("bytes", totalBytes),
			zap.Float64("seconds", elapsed),
			zap.Float64("MB/s", float64(totalBytes)/elapsed/1048576.0))
	}
	return nil
}

// ProcessDumpDataWithDeduplication applies a dump stream to destPath using the given dedup strategy without checksum verification, updating the strategy's state.
func (t *Transfer) ProcessDumpDataWithDeduplication(cfg *config.Config, in io.Reader, destPath string, dedup DeduplicationStrategy) error {
	return t.processDumpDataCore(cfg, in, destPath, dedup, false)
}

// ProcessDumpData applies a dump stream to destPath with checksum verification for each block before writing.
func (t *Transfer) ProcessDumpData(cfg *config.Config, in io.Reader, destPath string) error {
	return t.processDumpDataCore(cfg, in, destPath, nil, true)
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

func (t *Transfer) applyData(cfg *config.Config, in io.Reader, destDevice string) error {
	dedup := NewDeduplicationStrategy(cfg, t.Logger)
	if dedup != nil {
		if t.Logger != nil {
			t.Logger.Info("Applying deduplication during restore", zap.String("strategy", cfg.DedupStrategy))
		}
		defer func() {
			if err := dedup.SaveState(); err != nil {
				if t.Logger != nil {
					t.Logger.Error("Failed to save dedup state", zap.Error(err))
				}
			}
		}()
		return t.ProcessDumpDataWithDeduplication(cfg, in, destDevice, dedup)
	}
	return t.ProcessDumpData(cfg, in, destDevice)
}

// RunApply reads a dump file or stdin and writes the data to destDevice.
func (t *Transfer) RunApply(cfg *config.Config, applyFile, destDevice string) (err error) {
	rc, err := openApplyReader(applyFile)
	if err != nil {
		return err
	}
	defer common.CloseWithErr(rc, &err, "close apply file")
	return t.applyData(cfg, rc, destDevice)
}
