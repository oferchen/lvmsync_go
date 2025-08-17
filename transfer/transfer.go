package transfer

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/common"
	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	manifestpkg "lvmsync_go/manifest"
)

// Transfer encapsulates transfer state shared across operations.
type Transfer struct {
	Logger   *zap.Logger
	workerWG *sync.WaitGroup
	Tracker  *resumeTracker
	Info     device.DeviceInfoProvider
}

// NewTransfer creates a Transfer with the provided logger and wait group.
// When wg is nil, a new instance is allocated. A nil logger is replaced with
// zap.NewNop().
func NewTransfer(logger *zap.Logger, wg *sync.WaitGroup, info device.DeviceInfoProvider) *Transfer {
	if logger == nil {
		logger = zap.NewNop()
	}
	if wg == nil {
		wg = &sync.WaitGroup{}
	}
	if info == nil {
		info = device.NewInfo()
	}
	return &Transfer{Logger: logger, workerWG: wg, Tracker: &resumeTracker{}, Info: info}
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
func (t *Transfer) dumpChangesCore(ctx context.Context, cfg *config.Config, snapshot, source string, out io.Writer, dedup DeduplicationStrategy, handshake string) (err error) {
	defer rootcmd.SyncLogger(t.Logger)

	ranges, err := prepareRanges(ctx, cfg, snapshot, source, t.Logger)
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
	totalBytesTransferred, skippedBlocks, finalDigest, err = iterateBlocks(ctx, cfg, ranges, srcFile, bufOut, dedup, pipeFds, t.Logger, t.Tracker)
	if err != nil {
		return err
	}
	finalizeProgress(cfg, t.Logger)

	logSequentialSummary(t.Logger, totalBytesTransferred, skippedBlocks, startTime)
	finalizeResumeState(cfg, t.Tracker, t.Logger)
	if len(finalDigest) > 0 {
		t.Logger.Info("final checksum", zap.String("final_digest", fmt.Sprintf("%x", finalDigest)))
	}
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
func (t *Transfer) DumpChangesSequential(ctx context.Context, cfg *config.Config, snapshot, source string, out io.Writer) error {
	dedup, cleanup := t.setupDedup(cfg)
	if dedup != nil {
		defer cleanup()
	}
	return t.dumpChangesCore(ctx, cfg, snapshot, source, out, dedup, "")
}

// DumpChangesWithDeduplication transfers changed blocks using the provided dedup strategy and a checksum-dedup handshake, updating the strategy's state.
func (t *Transfer) DumpChangesWithDeduplication(ctx context.Context, cfg *config.Config, snapshot, source string, out io.Writer, dedup DeduplicationStrategy) error {
	return t.dumpChangesCore(ctx, cfg, snapshot, source, out, dedup, "checksum-dedup")
}

// DumpChanges chooses an appropriate transfer mode and persists dedup state when a strategy is configured.
func (t *Transfer) DumpChanges(ctx context.Context, cfg *config.Config, snapshot, source string, out io.Writer) error {
	dedup, cleanup := t.setupDedup(cfg)
	if dedup != nil {
		defer cleanup()
		t.Logger.Info("Deduplication enabled", zap.String("strategy", cfg.DedupStrategy))
		return t.DumpChangesWithDeduplication(ctx, cfg, snapshot, source, out, dedup)
	}
	t.Logger.Info("Deduplication disabled, performing full block transfer")
	return t.DumpChangesSequential(ctx, cfg, snapshot, source, out)
}

func manifestHeaderMAC(h *manifestpkg.Header) [32]byte {
	var buf [manifestpkg.HeaderSize - 32]byte
	binary.LittleEndian.PutUint32(buf[0:4], h.Version)
	binary.LittleEndian.PutUint32(buf[4:8], h.BlockSize)
	binary.LittleEndian.PutUint64(buf[8:16], h.SizeBytes)
	binary.LittleEndian.PutUint64(buf[16:24], h.ChunkCount)
	binary.LittleEndian.PutUint32(buf[24:28], h.MinChunkSize)
	binary.LittleEndian.PutUint32(buf[28:32], h.AvgChunkSize)
	binary.LittleEndian.PutUint32(buf[32:36], h.MaxChunkSize)
	binary.LittleEndian.PutUint32(buf[36:40], h.HybridFixedSize)
	copy(buf[40:], h.DeviceID[:])
	return blake3.Sum256(buf[:])
}

func readManifestHeader(ctx context.Context, path string, timeout time.Duration) (*manifestpkg.Header, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	f, err := common.OpenWithContext(ctx, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var hdr manifestpkg.Header
	if err := binary.Read(f, binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("read manifest header: %w", err)
	}
	if mac := manifestHeaderMAC(&hdr); !bytes.Equal(mac[:], hdr.MAC[:]) {
		return nil, fmt.Errorf("manifest: header MAC mismatch")
	}
	if hdr.Version != manifestpkg.Version {
		return nil, fmt.Errorf("manifest: version mismatch")
	}
	return &hdr, nil
}

func (t *Transfer) verifyDestination(ctx context.Context, cfg *config.Config, destPath string) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	if cfg.ManifestPath != "" {
		hdr, err := readManifestHeader(ctx, cfg.ManifestPath, 0)
		if err != nil {
			return err
		}
		id, err := t.Info.GetDeviceID(ctx, destPath)
		if err != nil {
			return fmt.Errorf("read destination id: %w", err)
		}
		manID := strings.TrimRight(string(hdr.DeviceID[:]), "\x00")
		if id != manID {
			t.Logger.Error("device_id_mismatch", zap.String("expected_resource_id", manID), zap.String("resource_id", id))
			return fmt.Errorf("destination device id %s does not match manifest %s", id, manID)
		}
		size, err := device.SizeBytes(ctx, destPath)
		if err != nil {
			return fmt.Errorf("read destination size: %w", err)
		}
		if size != hdr.SizeBytes {
			t.Logger.Error("device_size_mismatch", zap.Uint64("expected_size_bytes", hdr.SizeBytes), zap.Uint64("size_bytes", size))
			return fmt.Errorf("destination device size %d does not match manifest %d", size, hdr.SizeBytes)
		}
		t.Logger.Info("destination_validated", zap.String("resource_id", id), zap.Uint64("size_bytes", size))
	} else if cfg.DeviceUUID != "" {
		id, err := t.Info.GetDeviceID(ctx, destPath)
		if err != nil {
			return fmt.Errorf("read destination uuid: %w", err)
		}
		if id != cfg.DeviceUUID {
			t.Logger.Error("device_id_mismatch", zap.String("expected_resource_id", cfg.DeviceUUID), zap.String("resource_id", id))
			return fmt.Errorf("destination device uuid %s does not match expected %s", id, cfg.DeviceUUID)
		}
		t.Logger.Info("destination_validated", zap.String("resource_id", id))
	}
	mounted, err := device.IsMountedRW(ctx, destPath)
	if err != nil {
		return fmt.Errorf("check mount status: %w", err)
	}
	if mounted && !cfg.Force {
		t.Logger.Error("destination_mounted_rw", zap.String("path", destPath))
		return fmt.Errorf("destination device %s is mounted read-write", destPath)
	}
	return nil
}
