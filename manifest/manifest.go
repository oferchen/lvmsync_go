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
	Version uint32 = 1

	headerSize = 4 + 4 + 8 + 8 + 64 + 32 // 120 bytes
	entrySize  = 8 + 4 + 4 + 8 + 32      // 56 bytes
)

// Header describes the device tracked by the manifest.
// It is stored in little-endian binary form at the start of the file.
// The MAC field is the BLAKE3 digest of the preceding header fields.
type Header struct {
	Version    uint32
	BlockSize  uint32
	SizeBytes  uint64
	ChunkCount uint64
	DeviceID   [64]byte
	MAC        [32]byte
}

// Index is an mmap-backed manifest file containing chunk metadata.
type Index struct {
	f    *os.File
	data []byte
	hdr  Header
}

var closeHook = func() {}

var detectDevice = func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error) {
	return device.Detect(ctx, path, true, "", "", 0, 0, logger)
}

// ErrVersionMismatch is returned when a manifest file uses an unsupported version.
var ErrVersionMismatch = errors.New("manifest: version mismatch")

func headerMAC(h *Header) [32]byte {
	var buf [headerSize - 32]byte
	binary.LittleEndian.PutUint32(buf[0:4], h.Version)
	binary.LittleEndian.PutUint32(buf[4:8], h.BlockSize)
	binary.LittleEndian.PutUint64(buf[8:16], h.SizeBytes)
	binary.LittleEndian.PutUint64(buf[16:24], h.ChunkCount)
	copy(buf[24:], h.DeviceID[:])
	sum := blake3.Sum256(buf[:])
	return sum
}

func (i *Index) writeHeader() {
	var buf [headerSize]byte
	binary.LittleEndian.PutUint32(buf[0:4], i.hdr.Version)
	binary.LittleEndian.PutUint32(buf[4:8], i.hdr.BlockSize)
	binary.LittleEndian.PutUint64(buf[8:16], i.hdr.SizeBytes)
	binary.LittleEndian.PutUint64(buf[16:24], i.hdr.ChunkCount)
	copy(buf[24:88], i.hdr.DeviceID[:])
	copy(buf[88:120], i.hdr.MAC[:])
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
	copy(i.hdr.DeviceID[:], buf[24:88])
	copy(i.hdr.MAC[:], buf[88:120])
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
	defer closeHook()
	if i.data != nil {
		if err := unix.Msync(i.data, unix.MS_SYNC); err != nil {
			_ = unix.Munmap(i.data)
			_ = i.f.Close()
			return err
		}
		_ = unix.Munmap(i.data)
	}
	if i.f != nil {
		return i.f.Close()
	}
	return nil
}

// Create initializes a new manifest index at path for the given device.
func Create(path, deviceID string, size uint64, blockSize uint32) (*Index, error) {
	if len(deviceID) > 64 {
		return nil, fmt.Errorf("manifest: device ID exceeds 64 bytes")
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
	idx := &Index{f: f, data: data}
	idx.hdr = Header{
		Version:    Version,
		BlockSize:  blockSize,
		SizeBytes:  size,
		ChunkCount: chunkCount,
	}
	copy(idx.hdr.DeviceID[:], []byte(deviceID))
	idx.hdr.MAC = headerMAC(&idx.hdr)
	idx.writeHeader()
	return idx, nil
}

// Open maps an existing manifest index file.
func Open(path string) (*Index, error) {
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
	idx := &Index{f: f, data: data}
	if err := idx.readHeader(); err != nil {
		idx.Close()
		return nil, err
	}
	return idx, nil
}

// Upgrade opens the manifest at path, upgrading older versions in-place.
// It returns an Index mapped to the upgraded file.
func Upgrade(path string) (*Index, error) {
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
	idx := &Index{f: f, data: data}
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
func (i *Index) Set(offset uint64, length uint32, xxh uint64, digest [32]byte) {
	idx := offset / uint64(i.hdr.BlockSize)
	off := entryOffset(idx)
	start := int(off)
	binary.LittleEndian.PutUint64(i.data[start:start+8], offset)
	binary.LittleEndian.PutUint32(i.data[start+8:start+12], length)
	// 4 bytes padding at start+12:start+16
	binary.LittleEndian.PutUint64(i.data[start+16:start+24], xxh)
	copy(i.data[start+24:start+56], digest[:])
}

// Match reports whether the manifest already has a record for the chunk at the
// given offset and length. The provided digestFn is invoked to compute the
// BLAKE3 digest only if the stored XXH3 hash matches the supplied xxh.
func (i *Index) Match(offset uint64, length uint32, xxh uint64, digestFn func() [32]byte) bool {
	idx := offset / uint64(i.hdr.BlockSize)
	if idx >= i.hdr.ChunkCount {
		return false
	}
	off := entryOffset(idx)
	start := int(off)
	storedOffset := binary.LittleEndian.Uint64(i.data[start : start+8])
	storedLen := binary.LittleEndian.Uint32(i.data[start+8 : start+12])
	if storedLen == 0 {
		return false
	}
	if storedOffset != offset || storedLen != length {
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
func (i *Index) Entry(idx uint64) (offset uint64, length uint32, xxh uint64, digest [32]byte) {
	off := entryOffset(idx)
	start := int(off)
	offset = binary.LittleEndian.Uint64(i.data[start : start+8])
	length = binary.LittleEndian.Uint32(i.data[start+8 : start+12])
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
func Rebuild(ctx context.Context, devicePath, output string, logger *zap.Logger, progressInterval time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dev, err := detectDevice(ctx, devicePath, logger)
	if err != nil {
		return err
	}
	defer dev.Close()
	blockSize := uint32(dev.BlockSize())
	size := dev.SizeBytes()
	if err := ctx.Err(); err != nil {
		return err
	}
	id, err := device.GetUUID(ctx, dev.Path())
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.Open(dev.Path())
	if err != nil {
		return err
	}
	defer f.Close()
	idx, err := Create(output, id, size, blockSize)
	if err != nil {
		return err
	}
	defer idx.Close()
	start := time.Now()
	lastLog := start
	buf := make([]byte, blockSize)
	for off := uint64(0); off < size; off += uint64(blockSize) {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := f.ReadAt(buf, int64(off))
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}
		data := buf[:n]
		xx := xxh3.Hash(data)
		b3 := blake3.Sum256(data)
		idx.Set(off, uint32(n), xx, b3)
		if logger != nil && (progressInterval == 0 || time.Since(lastLog) >= progressInterval) {
			if err := ctx.Err(); err != nil {
				return err
			}
			logger.Info("rebuild progress",
				zap.Uint64("offset_bytes", off+uint64(n)),
				zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
			lastLog = time.Now()
		}
		if err == io.EOF {
			break
		}
	}
	return nil
}
