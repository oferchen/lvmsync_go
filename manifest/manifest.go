package manifest

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"os"
	"time"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/device"
	"lvmsync_go/internal/privilege"
)

const (
	// Version identifies the manifest format.
	Version uint32 = 2

	// HeaderSize is the binary size of Header.
	HeaderSize = 4 + 4 + 8 + 8 + 4 + 4 + 4 + 4 + 8 + 4 + 4 + 64 + 32 + 32 // 184 bytes
	entrySize  = 8 + 4 + 4 + 8 + 32                                       // 56 bytes

	// FlagCDC marks chunks produced by content-defined chunking.
	FlagCDC uint32 = 1 << 0

	// bloomRange is the size in bytes of each range tracked by a Bloom filter.
	bloomRange           = 1 << 20 // 1 MiB
	firstBlockDigestSize = 1 << 20 // 1 MiB

	emptySlot = ^uint64(0)
)

// Header describes the device tracked by the manifest.
// It is stored in little-endian binary form at the start of the file.
// The MAC field is the BLAKE3 digest of the preceding header fields.
type Header struct {
	Version          uint32
	BlockSize        uint32
	SizeBytes        uint64
	ChunkCount       uint64
	MinChunkSize     uint32
	AvgChunkSize     uint32
	MaxChunkSize     uint32
	HybridFixedSize  uint32
	Epoch            uint64
	Major            uint32
	Minor            uint32
	DeviceID         [64]byte
	FirstBlockDigest [32]byte
	MAC              [32]byte
}

// Index is an mmap-backed manifest file containing chunk metadata.
type Index struct {
	f         *os.File
	data      []byte
	hdr       Header
	closeHook func() error

	table []uint64
	bloom []uint64
	mask  uint64

	path    string
	tmpPath string
}

// indexOptions collects constructor options for Index and helpers like Rebuild.
type indexOptions struct {
	detectDevice func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error)
	closeHook    func() error
	info         device.DeviceInfoProvider
}

// IndexOption configures construction of an Index.
type IndexOption func(*indexOptions)

// defaultIndexOptions returns the default option set.
func defaultIndexOptions() indexOptions {
	return indexOptions{
		detectDevice: func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error) {
			ctx = device.WithForce(ctx, true)
			ctx = device.WithAllowOverwrite(ctx, true)
			ctx = device.WithYesIKnow(ctx, true)
			return device.Detect(ctx, path, true, "", "", "", "", 0, 0, privilege.New(ctx), logger, device.NewRunner())
		},
		closeHook: func() error { return nil },
		info:      device.NewInfo(),
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

// WithDeviceInfo overrides device information helpers used by Rebuild.
func WithDeviceInfo(info device.DeviceInfoProvider) IndexOption {
	return func(o *indexOptions) { o.info = info }
}

// ErrVersionMismatch is returned when a manifest file uses an unsupported version.
// ErrIndexOutOfRange is returned when a chunk index is outside the manifest range.
var (
	ErrVersionMismatch = errors.New("manifest: version mismatch")
	ErrIndexOutOfRange = errors.New("manifest: chunk index out of range")
)

func headerMAC(h *Header) [32]byte {
	var buf [HeaderSize - 32]byte
	binary.LittleEndian.PutUint32(buf[0:4], h.Version)
	binary.LittleEndian.PutUint32(buf[4:8], h.BlockSize)
	binary.LittleEndian.PutUint64(buf[8:16], h.SizeBytes)
	binary.LittleEndian.PutUint64(buf[16:24], h.ChunkCount)
	binary.LittleEndian.PutUint32(buf[24:28], h.MinChunkSize)
	binary.LittleEndian.PutUint32(buf[28:32], h.AvgChunkSize)
	binary.LittleEndian.PutUint32(buf[32:36], h.MaxChunkSize)
	binary.LittleEndian.PutUint32(buf[36:40], h.HybridFixedSize)
	binary.LittleEndian.PutUint64(buf[40:48], h.Epoch)
	binary.LittleEndian.PutUint32(buf[48:52], h.Major)
	binary.LittleEndian.PutUint32(buf[52:56], h.Minor)
	copy(buf[56:120], h.DeviceID[:])
	copy(buf[120:152], h.FirstBlockDigest[:])
	sum := blake3.Sum256(buf[:])
	return sum
}

func (i *Index) writeHeader() {
	var buf [HeaderSize]byte
	binary.LittleEndian.PutUint32(buf[0:4], i.hdr.Version)
	binary.LittleEndian.PutUint32(buf[4:8], i.hdr.BlockSize)
	binary.LittleEndian.PutUint64(buf[8:16], i.hdr.SizeBytes)
	binary.LittleEndian.PutUint64(buf[16:24], i.hdr.ChunkCount)
	binary.LittleEndian.PutUint32(buf[24:28], i.hdr.MinChunkSize)
	binary.LittleEndian.PutUint32(buf[28:32], i.hdr.AvgChunkSize)
	binary.LittleEndian.PutUint32(buf[32:36], i.hdr.MaxChunkSize)
	binary.LittleEndian.PutUint32(buf[36:40], i.hdr.HybridFixedSize)
	binary.LittleEndian.PutUint64(buf[40:48], i.hdr.Epoch)
	binary.LittleEndian.PutUint32(buf[48:52], i.hdr.Major)
	binary.LittleEndian.PutUint32(buf[52:56], i.hdr.Minor)
	copy(buf[56:120], i.hdr.DeviceID[:])
	copy(buf[120:152], i.hdr.FirstBlockDigest[:])
	copy(buf[152:184], i.hdr.MAC[:])
	copy(i.data[:HeaderSize], buf[:])
}

func (i *Index) readHeader() error {
	if len(i.data) < HeaderSize {
		return fmt.Errorf("manifest: file too small")
	}
	buf := i.data[:HeaderSize]
	i.hdr.Version = binary.LittleEndian.Uint32(buf[0:4])
	i.hdr.BlockSize = binary.LittleEndian.Uint32(buf[4:8])
	i.hdr.SizeBytes = binary.LittleEndian.Uint64(buf[8:16])
	i.hdr.ChunkCount = binary.LittleEndian.Uint64(buf[16:24])
	i.hdr.MinChunkSize = binary.LittleEndian.Uint32(buf[24:28])
	i.hdr.AvgChunkSize = binary.LittleEndian.Uint32(buf[28:32])
	i.hdr.MaxChunkSize = binary.LittleEndian.Uint32(buf[32:36])
	i.hdr.HybridFixedSize = binary.LittleEndian.Uint32(buf[36:40])
	i.hdr.Epoch = binary.LittleEndian.Uint64(buf[40:48])
	i.hdr.Major = binary.LittleEndian.Uint32(buf[48:52])
	i.hdr.Minor = binary.LittleEndian.Uint32(buf[52:56])
	copy(i.hdr.DeviceID[:], buf[56:120])
	copy(i.hdr.FirstBlockDigest[:], buf[120:152])
	copy(i.hdr.MAC[:], buf[152:184])
	if mac := headerMAC(&i.hdr); !bytes.Equal(mac[:], i.hdr.MAC[:]) {
		return fmt.Errorf("manifest: header MAC mismatch")
	}
	if i.hdr.Version != Version {
		return ErrVersionMismatch
	}
	return nil
}

func (i *Index) validateSize() error {
	expected := uint64(HeaderSize) + i.hdr.ChunkCount*uint64(entrySize)
	if uint64(len(i.data)) != expected {
		return fmt.Errorf("manifest: unexpected file size: got %d, want %d", len(i.data), expected)
	}
	return nil
}

// Close flushes the mmap and closes the underlying file.
func (i *Index) Close() error {
	var err error
	if i.data != nil {
		if syncErr := unix.Msync(i.data, unix.MS_SYNC); syncErr != nil {
			err = multierr.Append(err, syncErr)
		}
		if unmapErr := unix.Munmap(i.data); unmapErr != nil {
			err = multierr.Append(err, unmapErr)
		}
	}
	if i.f != nil {
		if syncErr := i.f.Sync(); syncErr != nil {
			err = multierr.Append(err, syncErr)
		}
		if ferr := i.f.Close(); ferr != nil {
			err = multierr.Append(err, ferr)
		}
		if i.tmpPath != "" {
			if renErr := fsyncRename(i.tmpPath, i.path); renErr != nil {
				err = multierr.Append(err, renErr)
			}
		}
	}
	if i.closeHook != nil {
		if hookErr := i.closeHook(); hookErr != nil {
			err = multierr.Append(err, hookErr)
		}
	}
	return err
}

// Create initializes a new manifest index at path for the given device.
func Create(path, deviceID string, size, epoch uint64, major, minor uint32, blockSize, cdcMin, cdcAvg, cdcMax, hybridFixed uint32, opts ...IndexOption) (*Index, error) {
	cfg := applyOptions(opts)
	if len(deviceID) > 64 {
		return nil, fmt.Errorf("manifest: device ID exceeds 64 bytes")
	}
	if blockSize == 0 {
		return nil, fmt.Errorf("manifest: block size must be greater than zero")
	}
	chunkCount := (size + uint64(blockSize) - 1) / uint64(blockSize)
	total := HeaderSize + entrySize*chunkCount
	f, data, tmp, err := openTemp(path, int64(total))
	if err != nil {
		return nil, err
	}
	idx := &Index{f: f, data: data, closeHook: cfg.closeHook, path: path, tmpPath: tmp}
	idx.hdr = Header{
		Version:         Version,
		BlockSize:       blockSize,
		SizeBytes:       size,
		ChunkCount:      chunkCount,
		MinChunkSize:    cdcMin,
		AvgChunkSize:    cdcAvg,
		MaxChunkSize:    cdcMax,
		HybridFixedSize: hybridFixed,
		Epoch:           epoch,
		Major:           major,
		Minor:           minor,
	}
	copy(idx.hdr.DeviceID[:], []byte(deviceID))
	idx.hdr.MAC = headerMAC(&idx.hdr)
	idx.writeHeader()
	idx.initTables()
	if err := unix.Msync(idx.data[:HeaderSize], unix.MS_SYNC); err != nil {
		idx.Close()
		return nil, err
	}
	if err := f.Sync(); err != nil {
		idx.Close()
		return nil, err
	}
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
	if err := idx.validateSize(); err != nil {
		idx.Close()
		return nil, err
	}
	idx.buildTables()
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
	if err := idx.validateSize(); err != nil {
		idx.Close()
		return nil, err
	}
	idx.buildTables()
	return idx, nil
}

func entryOffset(i uint64) uint64 { return uint64(HeaderSize) + i*entrySize }

func nextPow2(v uint64) uint64 {
	if v == 0 {
		return 1
	}
	return 1 << (64 - bits.LeadingZeros64(v-1))
}

func (i *Index) initTables() {
	size := nextPow2(i.hdr.ChunkCount * 2)
	i.table = make([]uint64, size)
	for j := range i.table {
		i.table[j] = emptySlot
	}
	i.mask = uint64(size - 1)
	ranges := (i.hdr.SizeBytes + bloomRange - 1) / bloomRange
	i.bloom = make([]uint64, ranges)
}

func (i *Index) insert(offset, xxh, idx uint64) {
	if len(i.table) == 0 {
		return
	}
	pos := xxh & i.mask
	for {
		if i.table[pos] == emptySlot {
			i.table[pos] = idx
			break
		}
		pos = (pos + 1) & i.mask
	}
	r := offset / bloomRange
	bit1 := xxh & 63
	bit2 := (xxh >> 6) & 63
	i.bloom[r] |= (1<<bit1 | 1<<bit2)
}

func (i *Index) buildTables() {
	i.initTables()
	for idx := uint64(0); idx < i.hdr.ChunkCount; idx++ {
		off := entryOffset(idx)
		start := int(off)
		length := binary.LittleEndian.Uint32(i.data[start+8 : start+12])
		if length == 0 {
			continue
		}
		offset := binary.LittleEndian.Uint64(i.data[start : start+8])
		xxh := binary.LittleEndian.Uint64(i.data[start+16 : start+24])
		i.insert(offset, xxh, idx)
	}
}

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
	i.insert(offset, xxh, idx)
	return nil
}

// Match reports whether the manifest already has a record for the chunk at the
// given offset, length, and flags. The provided digestFn is invoked to compute
// the BLAKE3 digest only after Bloom and XXH3 checks succeed.
func (i *Index) Match(offset uint64, length, flags uint32, xxh uint64, digestFn func() [32]byte) bool {
	if len(i.table) == 0 {
		return false
	}
	r := offset / bloomRange
	if r >= uint64(len(i.bloom)) {
		return false
	}
	bit1 := xxh & 63
	bit2 := (xxh >> 6) & 63
	mask := (uint64(1) << bit1) | (uint64(1) << bit2)
	if i.bloom[r]&mask != mask {
		return false
	}
	pos := xxh & i.mask
	for {
		idx := i.table[pos]
		if idx == emptySlot {
			return false
		}
		off := entryOffset(idx)
		start := int(off)
		storedXXH := binary.LittleEndian.Uint64(i.data[start+16 : start+24])
		if storedXXH == xxh {
			storedOffset := binary.LittleEndian.Uint64(i.data[start : start+8])
			storedLen := binary.LittleEndian.Uint32(i.data[start+8 : start+12])
			storedFlags := binary.LittleEndian.Uint32(i.data[start+12 : start+16])
			if storedOffset == offset && storedLen == length && storedFlags == flags {
				digest := digestFn()
				return bytes.Equal(i.data[start+24:start+56], digest[:])
			}
		}
		pos = (pos + 1) & i.mask
	}
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

// Chunk describes a CDC chunk recorded in the manifest.
type Chunk struct {
	Offset uint64
	Length uint32
	Digest [32]byte
}

// CDCChunks returns all CDC chunks stored in the manifest.
func (i *Index) CDCChunks() []Chunk {
	out := make([]Chunk, 0, i.hdr.ChunkCount)
	for idx := uint64(0); idx < i.hdr.ChunkCount; idx++ {
		offset, length, flags, _, digest, err := i.Entry(idx)
		if err != nil {
			continue
		}
		if flags&FlagCDC == 0 || length == 0 {
			continue
		}
		out = append(out, Chunk{Offset: offset, Length: length, Digest: digest})
	}
	return out
}

// Rebuild creates a manifest index for device at output path.
// DeviceID is determined via the configured DeviceInfoProvider. The device is read sequentially using blockSize-sized chunks.
// Progress is logged at the provided interval; set interval to 0 to log every chunk.
// The operation respects cancellation via ctx. When allowMounted is false,
// Rebuild aborts if the device is mounted read-write. logger must be non-nil;
// pass zap.NewNop() to disable logging.
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
	if err = ctx.Err(); err != nil {
		return err
	}
	mounted, err := cfg.info.IsMountedRW(ctx, devicePath)
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
	id, err := cfg.info.GetUUID(ctx, dev.Path())
	if err != nil {
		return err
	}
	digest, err := cfg.info.FirstBlockDigest(ctx, dev.Path(), firstBlockDigestSize)
	if err != nil {
		return err
	}
	identity, err := dev.Identity(ctx)
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
	epoch := uint64(time.Now().UnixNano())
	idx, err = Create(output, id, size, epoch, identity.Major, identity.Minor, blockSize, cdcMin, cdcAvg, cdcMax, hybridFixed, opts...)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := idx.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()
	copy(idx.hdr.FirstBlockDigest[:], digest[:])
	idx.hdr.MAC = headerMAC(&idx.hdr)
	idx.writeHeader()
	if err := unix.Msync(idx.data[:HeaderSize], unix.MS_SYNC); err != nil {
		return err
	}
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
