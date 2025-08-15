package transfer

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/config"
)

// Transfer encapsulates transfer state shared across operations.
type Transfer struct {
	Logger   *zap.Logger
	workerWG *sync.WaitGroup
	Tracker  *resumeTracker
}

// NewTransfer creates a Transfer with the provided logger and wait group.
// When wg is nil, a new instance is allocated. logger must be non-nil.
func NewTransfer(logger *zap.Logger, wg *sync.WaitGroup) *Transfer {
	if wg == nil {
		wg = &sync.WaitGroup{}
	}
	return &Transfer{Logger: logger, workerWG: wg, Tracker: &resumeTracker{}}
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

// SaveChecksumState persists block checksums; logger must be non-nil.
//
//revive:disable-next-line:cognitive-complexity
func SaveChecksumState(filename string, state *ChecksumState, logger *zap.Logger) (err error) {
	var file *os.File
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("create checksum state: %w", err)
	}
	if err = file.Chmod(0o600); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			logger.Warn("Failed to close checksum state file", zap.Error(closeErr))
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

// dumpChangesCore handles core transfer logic; logger must be non-nil.
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

	checkpoint := readResumeState(cfg, t.Logger)
	startIdx := findResumeIndex(cfg, srcFile, ranges, checkpoint, t.Logger)
	if startIdx > 0 {
		ranges = ranges[startIdx:]
	}

	startTime := time.Now()
	var totalBytesTransferred int64
	var skippedBlocks int
	var finalDigest []byte
	totalBytesTransferred, skippedBlocks, finalDigest, err = iterateBlocks(cfg, ranges, srcFile, bufOut, dedup, pipeFds, t.Logger, t.Tracker)
	if err != nil {
		return err
	}
	finalizeProgress(cfg, t.Logger)

	logSequentialSummary(t.Logger, totalBytesTransferred, skippedBlocks, startTime)
	finalizeResumeState(cfg, t.Tracker, t.Logger)
	if len(finalDigest) > 0 {
		t.Logger.Info("final checksum", zap.String("final_digest", fmt.Sprintf("%x", finalDigest)))
	}
	_ = t.Logger.Sync()
	return nil
}

func (t *Transfer) setupDedup(cfg *config.Config) (DeduplicationStrategy, func()) {
	dedup := NewDeduplicationStrategy(cfg, t.Logger)
	cleanup := func() {}
	if dedup != nil {
		cleanup = func() {
			if err := dedup.SaveState(); err != nil {
				t.Logger.Error("Failed to save dedup state", zap.Error(err))
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
		t.Logger.Info("Deduplication enabled", zap.String("strategy", cfg.DedupStrategy))
		return t.DumpChangesWithDeduplication(cfg, snapshot, source, out, dedup)
	}
	t.Logger.Info("Deduplication disabled, performing full block transfer")
	return t.DumpChangesSequential(cfg, snapshot, source, out)
}
