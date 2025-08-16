package manifest

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/device"
)

const (
	// Version identifies the manifest format.
	Version uint32 = 2

	headerSize = 4 + 4 + 8 + 8 + 4 + 4 + 4 + 4 + 64 + 32 // 136 bytes
	entrySize  = 8 + 4 + 4 + 8 + 32                      // 56 bytes

	// FlagCDC marks chunks produced by content-defined chunking.
	FlagCDC uint32 = 1 << 0
)

// Header describes the device tracked by the manifest.
// It is stored in little-endian binary form at the start of the file.
// The MAC field is the BLAKE3 digest of the preceding header fields.
type Header struct {
	Version         uint32
	BlockSize       uint32
	SizeBytes       uint64
	ChunkCount      uint64
	MinChunkSize    uint32
	AvgChunkSize    uint32
	MaxChunkSize    uint32
	HybridFixedSize uint32
	DeviceID        [64]byte
	MAC             [32]byte
}

// Index is an mmap-backed manifest file containing chunk metadata.
type Index struct {
	f         *os.File
	data      []byte
	hdr       Header
	closeHook func() error
}

// indexOptions collects constructor options for Index and helpers like Rebuild.
type indexOptions struct {
	detectDevice func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error)
	closeHook    func() error
}

// IndexOption configures construction of an Index.
type IndexOption func(*indexOptions)

// defaultIndexOptions returns the default option set.
func defaultIndexOptions() indexOptions {
	return indexOptions{
		detectDevice: func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error) {
			return device.Detect(ctx, path, true, "", "", "", "", 0, 0, logger)
		},
		closeHook: func() error { return nil },
	}
}

// applyOptions applies supplied options to the defaults.
func applyOptions(opts []IndexOption) indexOptions {
	cfg := defaultIndexOptions()
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithCloseHook sets a hook invoked when Index.Close is called.
func WithCloseHook(h func() error) IndexOption {
	return func(o *indexOptions) { o.closeHook = h }
}

// WithDetectDevice overrides device detection for Rebuild.
func WithDetectDevice(f func(context.Context, string, *zap.Logger) (device.Device, error)) IndexOption {
	return func(o *indexOptions) { o.detectDevice = f }
}

// ErrVersionMismatch is returned when a manifest file uses an unsupported version.
// ErrIndexOutOfRange is returned when a chunk index is outside the manifest range.
var (
	ErrVersionMismatch = errors.New("manifest: version mismatch")
	ErrIndexOutOfRange = errors.New("manifest: chunk index out of range")
)

func headerMAC(h *Header) [32]byte {
	var buf [headerSize - 32]byte
	binary.LittleEndian.PutUint32(buf[0:4], h.Version)
	binary.LittleEndian.PutUint32(buf[4:8], h.BlockSize)
	binary.LittleEndian.PutUint64(buf[8:16], h.SizeBytes)
	binary.LittleEndian.PutUint64(buf[16:24], h.ChunkCount)
	binary.LittleEndian.PutUint32(buf[24:28], h.MinChunkSize)
	binary.LittleEndian.PutUint32(buf[28:32], h.AvgChunkSize)
	binary.LittleEndian.PutUint32(buf[32:36], h.MaxChunkSize)
	binary.LittleEndian.PutUint32(buf[36:40], h.HybridFixedSize)
	copy(buf[40:], h.DeviceID[:])
	sum := blake3.Sum256(buf[:])
	return sum
}

func (i *Index) writeHeader() {
	var buf [headerSize]byte
	binary.LittleEndian.PutUint32(buf[0:4], i.hdr.Version)
	binary.LittleEndian.PutUint32(buf[4:8], i.hdr.BlockSize)
	binary.LittleEndian.PutUint64(buf[8:16], i.hdr.SizeBytes)
	binary.LittleEndian.PutUint64(buf[16:24], i.hdr.ChunkCount)
	binary.LittleEndian.PutUint32(buf[24:28], i.hdr.MinChunkSize)
	binary.LittleEndian.PutUint32(buf[28:32], i.hdr.AvgChunkSize)
	binary.LittleEndian.PutUint32(buf[32:36], i.hdr.MaxChunkSize)
	binary.LittleEndian.PutUint32(buf[36:40], i.hdr.HybridFixedSize)
	copy(buf[40:104], i.hdr.DeviceID[:])
	copy(buf[104:136], i.hdr.MAC[:])
	copy(i.data[:headerSize], buf[:])
}

func (i *Index) readHeader() error {
	if len(i.data) < headerSize {
		return fmt.Errorf("manifest: file too small")
	}
	buf := i.data[:headerSize]
	i.hdr.Version = binary.LittleEndian.Uint32(buf[0:4])
	i.hdr.BlockSize = binary.LittleEndian.Uint32(buf[4:8])
	i.hdr.SizeBytes = binary.LittleEndian.Uint64(buf[8:16])
	i.hdr.ChunkCount = binary.LittleEndian.Uint64(buf[16:24])
	i.hdr.MinChunkSize = binary.LittleEndian.Uint32(buf[24:28])
	i.hdr.AvgChunkSize = binary.LittleEndian.Uint32(buf[28:32])
	i.hdr.MaxChunkSize = binary.LittleEndian.Uint32(buf[32:36])
	i.hdr.HybridFixedSize = binary.LittleEndian.Uint32(buf[36:40])
	copy(i.hdr.DeviceID[:], buf[40:104])
	copy(i.hdr.MAC[:], buf[104:136])
	if mac := headerMAC(&i.hdr); !bytes.Equal(mac[:], i.hdr.MAC[:]) {
		return fmt.Errorf("manifest: header MAC mismatch")
	}
	if i.hdr.Version != Version {
		return ErrVersionMismatch
	}
	return nil
}

// Close flushes the mmap and closes the underlying file.
func (i *Index) Close() error {
	var err error
	if i.data != nil {
		if err = unix.Msync(i.data, unix.MS_SYNC); err != nil {
			_ = unix.Munmap(i.data)
			_ = i.f.Close()
			if i.closeHook != nil {
				_ = i.closeHook()
			}
			return err
		}
		_ = unix.Munmap(i.data)
	}
	if i.f != nil {
		if ferr := i.f.Close(); ferr != nil {
			err = ferr
		}
	}
	if i.closeHook != nil {
		if hookErr := i.closeHook(); err == nil && hookErr != nil {
			err = hookErr
		}
	}
	return err
}

// Create initializes a new manifest index at path for the given device.
func Create(path, deviceID string, size uint64, blockSize, cdcMin, cdcAvg, cdcMax, hybridFixed uint32, opts ...IndexOption) (*Index, error) {
	cfg := applyOptions(opts)
	if len(deviceID) > 64 {
		return nil, fmt.Errorf("manifest: device ID exceeds 64 bytes")
	}
	if blockSize == 0 {
		return nil, fmt.Errorf("manifest: block size must be greater than zero")
	}
	chunkCount := (size + uint64(blockSize) - 1) / uint64(blockSize)
	total := headerSize + entrySize*chunkCount
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Truncate(int64(total)); err != nil {
		f.Close()
		return nil, err
	}
	data, err := unix.Mmap(int(f.Fd()), 0, int(total), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, err
	}
	idx := &Index{f: f, data: data, closeHook: cfg.closeHook}
	idx.hdr = Header{
		Version:         Version,
		BlockSize:       blockSize,
		SizeBytes:       size,
		ChunkCount:      chunkCount,
		MinChunkSize:    cdcMin,
		AvgChunkSize:    cdcAvg,
		MaxChunkSize:    cdcMax,
		HybridFixedSize: hybridFixed,
	}
	copy(idx.hdr.DeviceID[:], []byte(deviceID))
	idx.hdr.MAC = headerMAC(&idx.hdr)
	idx.writeHeader()
	return idx, nil
}

// Open maps an existing manifest index file.
func Open(path string, opts ...IndexOption) (*Index, error) {
	cfg := applyOptions(opts)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	size := int(st.Size())
	data, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, err
	}
	idx := &Index{f: f, data: data, closeHook: cfg.closeHook}
	if err := idx.readHeader(); err != nil {
		idx.Close()
		return nil, err
	}
	return idx, nil
}

// Upgrade opens the manifest at path, upgrading older versions in-place.
// It returns an Index mapped to the upgraded file.
func Upgrade(path string, opts ...IndexOption) (*Index, error) {
	cfg := applyOptions(opts)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	size := int(st.Size())
	data, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, err
	}
	idx := &Index{f: f, data: data, closeHook: cfg.closeHook}
	if err := idx.readHeader(); err != nil {
		if !errors.Is(err, ErrVersionMismatch) {
			idx.Close()
			return nil, err
		}
		switch idx.hdr.Version {
		case 0:
			idx.hdr.Version = Version
			idx.hdr.MAC = headerMAC(&idx.hdr)
			idx.writeHeader()
		default:
			idx.Close()
			return nil, fmt.Errorf("manifest: unsupported version %d", idx.hdr.Version)
		}
	}
	return idx, nil
}

func entryOffset(i uint64) uint64 { return uint64(headerSize) + i*entrySize }

// Set records metadata for the chunk at the given offset.
func (i *Index) Set(offset uint64, length, flags uint32, xxh uint64, digest [32]byte) error {
	idx := offset / uint64(i.hdr.BlockSize)
	if idx >= i.hdr.ChunkCount {
		return ErrIndexOutOfRange
	}
	off := entryOffset(idx)
	start := int(off)
	binary.LittleEndian.PutUint64(i.data[start:start+8], offset)
	binary.LittleEndian.PutUint32(i.data[start+8:start+12], length)
	binary.LittleEndian.PutUint32(i.data[start+12:start+16], flags)
	binary.LittleEndian.PutUint64(i.data[start+16:start+24], xxh)
	copy(i.data[start+24:start+56], digest[:])
	return nil
}

// Match reports whether the manifest already has a record for the chunk at the
// given offset, length, and flags. The provided digestFn is invoked to compute
// the BLAKE3 digest only if the stored XXH3 hash matches the supplied xxh.
func (i *Index) Match(offset uint64, length, flags uint32, xxh uint64, digestFn func() [32]byte) bool {
	idx := offset / uint64(i.hdr.BlockSize)
	if idx >= i.hdr.ChunkCount {
		return false
	}
	off := entryOffset(idx)
	start := int(off)
	storedOffset := binary.LittleEndian.Uint64(i.data[start : start+8])
	storedLen := binary.LittleEndian.Uint32(i.data[start+8 : start+12])
	storedFlags := binary.LittleEndian.Uint32(i.data[start+12 : start+16])
	if storedLen == 0 {
		return false
	}
	if storedOffset != offset || storedLen != length || storedFlags != flags {
		return false
	}
	storedXXH := binary.LittleEndian.Uint64(i.data[start+16 : start+24])
	if storedXXH != xxh {
		return false
	}
	digest := digestFn()
	return bytes.Equal(i.data[start+24:start+56], digest[:])
}

// Entry returns metadata for the chunk at idx.
func (i *Index) Entry(idx uint64) (offset uint64, length, flags uint32, xxh uint64, digest [32]byte, err error) {
	if idx >= i.hdr.ChunkCount {
		return 0, 0, 0, 0, digest, ErrIndexOutOfRange
	}
	off := entryOffset(idx)
	start := int(off)
	offset = binary.LittleEndian.Uint64(i.data[start : start+8])
	length = binary.LittleEndian.Uint32(i.data[start+8 : start+12])
	flags = binary.LittleEndian.Uint32(i.data[start+12 : start+16])
	xxh = binary.LittleEndian.Uint64(i.data[start+16 : start+24])
	copy(digest[:], i.data[start+24:start+56])
	return
}

// ChunkCount returns the number of chunks tracked by the manifest.
func (i *Index) ChunkCount() uint64 { return i.hdr.ChunkCount }

// Rebuild creates a manifest index for device at output path.
// DeviceID is determined via device.GetUUID. The device is read sequentially using blockSize-sized chunks.
// Progress is logged at the provided interval; set interval to 0 to log every chunk.
// The operation respects cancellation via ctx.
// When allowMounted is false, Rebuild aborts if the device is mounted read-write.
func Rebuild(
	ctx context.Context,
	devicePath, output string,
	logger *zap.Logger,
	progressInterval time.Duration,
	allowMounted bool,
	cdcMin, cdcAvg, cdcMax, hybridFixed uint32,
	opts ...IndexOption,
) (err error) {
	cfg := applyOptions(opts)
	if logger == nil {
		logger = zap.NewNop()
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	mounted, err := device.IsMountedRW(devicePath)
	if err != nil {
		return fmt.Errorf("manifest: check mount status: %w", err)
	}
	if mounted && !allowMounted {
		return fmt.Errorf("manifest: %s is mounted read-write; use --manifest-allow-mounted to override", devicePath)
	}

	dev, err := cfg.detectDevice(ctx, devicePath, logger)
	if err != nil {
		return err
	}
	defer dev.Close()
	blockSize := uint32(dev.BlockSize())
	size := dev.SizeBytes()
	if err = ctx.Err(); err != nil {
		return err
	}
	id, err := device.GetUUID(ctx, dev.Path())
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	var f *os.File
	f, err = os.Open(dev.Path())
	if err != nil {
		return err
	}
	defer f.Close()
	var idx *Index

	idx, err = Create(output, id, size, blockSize, cdcMin, cdcAvg, cdcMax, hybridFixed, opts...)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := idx.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()
	start := time.Now()
	lastLog := start
	buf := make([]byte, blockSize)
	for off := uint64(0); off < size; off += uint64(blockSize) {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, readErr := f.ReadAt(buf, int64(off))
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		if n == 0 {
			break
		}
		data := buf[:n]
		xx := xxh3.Hash(data)
		b3 := blake3.Sum256(data)
		if err = idx.Set(off, uint32(n), 0, xx, b3); err != nil {
			return err
		}
		if progressInterval == 0 || time.Since(lastLog) >= progressInterval {
			if err = ctx.Err(); err != nil {
				return err
			}
			logger.Info("rebuild progress",
				zap.Uint64("offset_bytes", off+uint64(n)),
				zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
			lastLog = time.Now()
		}
		if readErr == io.EOF {
			break
		}
	}
	logger.Info("rebuild_complete",
		zap.Uint64("size_bytes", size),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	)
	return err
}
