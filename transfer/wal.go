package transfer

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeebo/blake3"
	"lvmsync_go/device"
	walpkg "lvmsync_go/internal/wal"
)

const (
	walVersion      = 2
	walHeaderV0Size = 8 + 8 + 64 + 32
	walHeaderV1Size = 8 + walHeaderV0Size
	walHeaderSize   = 8 + 8 + 8 + 64 + 64 + 8 + 64 + 4 + 4 + 32
)

type walHeader struct {
	Version      uint64
	Size         uint64
	Epoch        uint64
	KernelUUID   [64]byte
	GPTUUID      [64]byte
	MBRSignature [8]byte
	FSUUID       [64]byte
	Major        uint32
	Minor        uint32
	MAC          [32]byte
}

type walHeaderV1 struct {
	Version uint64
	Size    uint64
	Epoch   uint64
	FSUUID  [64]byte
	MAC     [32]byte
}

type walHeaderV0 struct {
	Size   uint64
	Epoch  uint64
	FSUUID [64]byte
	MAC    [32]byte
}

// WAL is a write-ahead log of device ranges. Each 16-byte entry records a
// committed range and is fsynced before returning from Append. Callers must
// append entries in monotonically increasing order.
//
// On first creation the header is written and both the file and its parent
// directory are fsynced to ensure durability. When reopened after a crash,
// OpenWAL scans for the last complete entry and truncates any partially written
// tail. Close fsyncs the file and its parent directory to guarantee that the
// final state of the WAL is persistent.
type WAL struct {
	*walpkg.WAL
	header walHeader
}

type WALDeps = walpkg.Deps

func NewWALDeps() *WALDeps { return walpkg.NewDeps() }

func walHeaderMAC(h *walHeader) [32]byte {
	var buf [8 + 8 + 8 + 64 + 64 + 8 + 64 + 4 + 4]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Version)
	binary.LittleEndian.PutUint64(buf[8:16], h.Size)
	binary.LittleEndian.PutUint64(buf[16:24], h.Epoch)
	copy(buf[24:88], h.KernelUUID[:])
	copy(buf[88:152], h.GPTUUID[:])
	copy(buf[152:160], h.MBRSignature[:])
	copy(buf[160:224], h.FSUUID[:])
	binary.LittleEndian.PutUint32(buf[224:228], h.Major)
	binary.LittleEndian.PutUint32(buf[228:232], h.Minor)
	return blake3.Sum256(buf[:])
}

func walHeaderMACV1(h *walHeaderV1) [32]byte {
	var buf [8 + 8 + 8 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Version)
	binary.LittleEndian.PutUint64(buf[8:16], h.Size)
	binary.LittleEndian.PutUint64(buf[16:24], h.Epoch)
	copy(buf[24:], h.FSUUID[:])
	return blake3.Sum256(buf[:])
}

func walHeaderMACV0(h *walHeaderV0) [32]byte {
	var buf [8 + 8 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Size)
	binary.LittleEndian.PutUint64(buf[8:16], h.Epoch)
	copy(buf[16:], h.FSUUID[:])
	return blake3.Sum256(buf[:])
}

// OpenWAL opens or creates the WAL at path. It validates the metadata and
// returns any fully committed ranges. If a crash left a partially written
// entry, the WAL is truncated to the last complete record and positioned for
// further appends.
func OpenWAL(path string, id device.DeviceIdentity, deps *WALDeps) (*WAL, []Range, error) {
	if deps == nil {
		deps = NewWALDeps()
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	w := &WAL{WAL: walpkg.New(f, deps)}
	if st.Size() == 0 {
		w.header.Version = walVersion
		w.header.Size = id.SizeBytes
		w.header.Epoch = id.ManifestEpoch
		copy(w.header.KernelUUID[:], []byte(id.KernelUUID))
		copy(w.header.GPTUUID[:], []byte(id.GPTUUID))
		copy(w.header.MBRSignature[:], []byte(id.MBRSignature))
		copy(w.header.FSUUID[:], []byte(id.FSUUID))
		w.header.Major = id.Major
		w.header.Minor = id.Minor
		w.header.MAC = walHeaderMAC(&w.header)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], w.header.Version)
		binary.LittleEndian.PutUint64(buf[8:16], w.header.Size)
		binary.LittleEndian.PutUint64(buf[16:24], w.header.Epoch)
		copy(buf[24:88], w.header.KernelUUID[:])
		copy(buf[88:152], w.header.GPTUUID[:])
		copy(buf[152:160], w.header.MBRSignature[:])
		copy(buf[160:224], w.header.FSUUID[:])
		binary.LittleEndian.PutUint32(buf[224:228], w.header.Major)
		binary.LittleEndian.PutUint32(buf[228:232], w.header.Minor)
		copy(buf[232:264], w.header.MAC[:])
		if n, err := f.WriteAt(buf[:], 0); err != nil {
			f.Close()
			return nil, nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if _, err := f.Seek(int64(walHeaderSize), 0); err != nil {
			f.Close()
			return nil, nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, nil, err
		}
		if err := deps.SyncDir(filepath.Dir(path)); err != nil {
			f.Close()
			return nil, nil, err
		}
		return w, nil, nil
	}

	var vbuf [8]byte
	if _, err := f.ReadAt(vbuf[:], 0); err != nil {
		f.Close()
		return nil, nil, err
	}
	ver := binary.LittleEndian.Uint64(vbuf[:])
	if ver == walVersion && st.Size() >= walHeaderSize {
		var buf [walHeaderSize]byte
		if _, err := f.ReadAt(buf[:], 0); err != nil {
			f.Close()
			return nil, nil, err
		}
		var hdr walHeader
		hdr.Version = binary.LittleEndian.Uint64(buf[0:8])
		hdr.Size = binary.LittleEndian.Uint64(buf[8:16])
		hdr.Epoch = binary.LittleEndian.Uint64(buf[16:24])
		copy(hdr.KernelUUID[:], buf[24:88])
		copy(hdr.GPTUUID[:], buf[88:152])
		copy(hdr.MBRSignature[:], buf[152:160])
		copy(hdr.FSUUID[:], buf[160:224])
		hdr.Major = binary.LittleEndian.Uint32(buf[224:228])
		hdr.Minor = binary.LittleEndian.Uint32(buf[228:232])
		copy(hdr.MAC[:], buf[232:264])
		if mac := walHeaderMAC(&hdr); mac != hdr.MAC {
			f.Close()
			return nil, nil, fmt.Errorf("wal: header mac mismatch")
		}
		hdrID := device.DeviceIdentity{
			SizeBytes:     hdr.Size,
			KernelUUID:    strings.TrimRight(string(hdr.KernelUUID[:]), "\x00"),
			GPTUUID:       strings.TrimRight(string(hdr.GPTUUID[:]), "\x00"),
			MBRSignature:  strings.TrimRight(string(hdr.MBRSignature[:]), "\x00"),
			FSUUID:        strings.TrimRight(string(hdr.FSUUID[:]), "\x00"),
			Major:         hdr.Major,
			Minor:         hdr.Minor,
			ManifestEpoch: hdr.Epoch,
		}
		if !device.SameIdentityStrict(hdrID, id) {
			f.Close()
			return nil, nil, fmt.Errorf("wal: metadata mismatch")
		}
		w.header = hdr
		ranges := []Range{}
		off := int64(walHeaderSize)
		entry := make([]byte, 16)
		for {
			n, err := f.ReadAt(entry, off)
			if err != nil || n != 16 {
				break
			}
			start := binary.LittleEndian.Uint64(entry[0:8])
			end := binary.LittleEndian.Uint64(entry[8:16])
			ranges = append(ranges, Range{Start: start, End: end})
			off += 16
		}
		if err := f.Truncate(off); err != nil {
			f.Close()
			return nil, nil, err
		}
		if _, err := f.Seek(off, 0); err != nil {
			f.Close()
			return nil, nil, err
		}
		return w, ranges, nil
	}

	if ver == 1 && st.Size() >= walHeaderV1Size {
		var buf1 [walHeaderV1Size]byte
		if _, err := f.ReadAt(buf1[:], 0); err != nil {
			f.Close()
			return nil, nil, err
		}
		var hdr1 walHeaderV1
		hdr1.Version = binary.LittleEndian.Uint64(buf1[0:8])
		hdr1.Size = binary.LittleEndian.Uint64(buf1[8:16])
		hdr1.Epoch = binary.LittleEndian.Uint64(buf1[16:24])
		copy(hdr1.FSUUID[:], buf1[24:88])
		copy(hdr1.MAC[:], buf1[88:120])
		if mac := walHeaderMACV1(&hdr1); mac != hdr1.MAC {
			f.Close()
			return nil, nil, fmt.Errorf("wal: header mac mismatch")
		}
		if hdr1.Size != id.SizeBytes || hdr1.Epoch != id.ManifestEpoch || strings.TrimRight(string(hdr1.FSUUID[:]), "\x00") != id.FSUUID {
			f.Close()
			return nil, nil, fmt.Errorf("wal: metadata mismatch")
		}
		data := make([]byte, st.Size()-walHeaderV1Size)
		if _, err := f.ReadAt(data, walHeaderV1Size); err != nil && err != io.EOF {
			f.Close()
			return nil, nil, err
		}
		ranges := []Range{}
		for off := 0; off+16 <= len(data); off += 16 {
			start := binary.LittleEndian.Uint64(data[off : off+8])
			end := binary.LittleEndian.Uint64(data[off+8 : off+16])
			ranges = append(ranges, Range{Start: start, End: end})
		}
		var hdr walHeader
		hdr.Version = walVersion
		hdr.Size = hdr1.Size
		hdr.Epoch = hdr1.Epoch
		copy(hdr.KernelUUID[:], []byte(id.KernelUUID))
		copy(hdr.GPTUUID[:], []byte(id.GPTUUID))
		copy(hdr.MBRSignature[:], []byte(id.MBRSignature))
		hdr.FSUUID = hdr1.FSUUID
		hdr.Major = id.Major
		hdr.Minor = id.Minor
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint64(buf[16:24], hdr.Epoch)
		copy(buf[24:88], hdr.KernelUUID[:])
		copy(buf[88:152], hdr.GPTUUID[:])
		copy(buf[152:160], hdr.MBRSignature[:])
		copy(buf[160:224], hdr.FSUUID[:])
		binary.LittleEndian.PutUint32(buf[224:228], hdr.Major)
		binary.LittleEndian.PutUint32(buf[228:232], hdr.Minor)
		copy(buf[232:264], hdr.MAC[:])
		if n, err := f.WriteAt(buf[:], 0); err != nil {
			f.Close()
			return nil, nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if len(data) > 0 {
			if n, err := f.WriteAt(data, walHeaderSize); err != nil {
				f.Close()
				return nil, nil, err
			} else if n != len(data) {
				f.Close()
				return nil, nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(data))
			}
		}
		if err := f.Truncate(int64(walHeaderSize + len(data))); err != nil {
			f.Close()
			return nil, nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, nil, err
		}
		w.header = hdr
		if _, err := f.Seek(int64(walHeaderSize+len(data)), 0); err != nil {
			f.Close()
			return nil, nil, err
		}
		return w, ranges, nil
	}

	if st.Size() >= walHeaderV0Size {
		var buf0 [walHeaderV0Size]byte
		if _, err := f.ReadAt(buf0[:], 0); err != nil {
			f.Close()
			return nil, nil, err
		}
		var hdr0 walHeaderV0
		hdr0.Size = binary.LittleEndian.Uint64(buf0[0:8])
		hdr0.Epoch = binary.LittleEndian.Uint64(buf0[8:16])
		copy(hdr0.FSUUID[:], buf0[16:80])
		copy(hdr0.MAC[:], buf0[80:112])
		if mac := walHeaderMACV0(&hdr0); mac != hdr0.MAC {
			f.Close()
			return nil, nil, fmt.Errorf("wal: header mac mismatch")
		}
		if hdr0.Size != id.SizeBytes || hdr0.Epoch != id.ManifestEpoch || strings.TrimRight(string(hdr0.FSUUID[:]), "\x00") != id.FSUUID {
			f.Close()
			return nil, nil, fmt.Errorf("wal: metadata mismatch")
		}
		data := make([]byte, st.Size()-walHeaderV0Size)
		if _, err := f.ReadAt(data, walHeaderV0Size); err != nil && err != io.EOF {
			f.Close()
			return nil, nil, err
		}
		ranges := []Range{}
		for off := 0; off+16 <= len(data); off += 16 {
			start := binary.LittleEndian.Uint64(data[off : off+8])
			end := binary.LittleEndian.Uint64(data[off+8 : off+16])
			ranges = append(ranges, Range{Start: start, End: end})
		}
		var hdr walHeader
		hdr.Version = walVersion
		hdr.Size = hdr0.Size
		hdr.Epoch = hdr0.Epoch
		copy(hdr.KernelUUID[:], []byte(id.KernelUUID))
		copy(hdr.GPTUUID[:], []byte(id.GPTUUID))
		copy(hdr.MBRSignature[:], []byte(id.MBRSignature))
		hdr.FSUUID = hdr0.FSUUID
		hdr.Major = id.Major
		hdr.Minor = id.Minor
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint64(buf[16:24], hdr.Epoch)
		copy(buf[24:88], hdr.KernelUUID[:])
		copy(buf[88:152], hdr.GPTUUID[:])
		copy(buf[152:160], hdr.MBRSignature[:])
		copy(buf[160:224], hdr.FSUUID[:])
		binary.LittleEndian.PutUint32(buf[224:228], hdr.Major)
		binary.LittleEndian.PutUint32(buf[228:232], hdr.Minor)
		copy(buf[232:264], hdr.MAC[:])
		if n, err := f.WriteAt(buf[:], 0); err != nil {
			f.Close()
			return nil, nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if len(data) > 0 {
			if n, err := f.WriteAt(data, walHeaderSize); err != nil {
				f.Close()
				return nil, nil, err
			} else if n != len(data) {
				f.Close()
				return nil, nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(data))
			}
		}
		if err := f.Truncate(int64(walHeaderSize + len(data))); err != nil {
			f.Close()
			return nil, nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, nil, err
		}
		w.header = hdr
		if _, err := f.Seek(int64(walHeaderSize+len(data)), 0); err != nil {
			f.Close()
			return nil, nil, err
		}
		return w, ranges, nil
	}

	f.Close()
	return nil, nil, fmt.Errorf("wal: file too small")
}

// Append records r in the WAL.
func (w *WAL) Append(r Range) error { return w.WAL.Append(r) }

// Sync flushes the WAL to stable storage.
func (w *WAL) Sync() error { return w.WAL.Sync() }

// Close flushes the WAL and fsyncs its parent directory.
func (w *WAL) Close() error { return w.WAL.Close() }
