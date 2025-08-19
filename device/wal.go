package device

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zeebo/blake3"
)

const (
	walVersion      = 2
	walHeaderV0Size = 8 + 64 + 64 + 64 + 32
	walHeaderV1Size = 8 + 8 + 64 + 64 + 64 + 32
	walHeaderSize   = 8 + 8 + 4 + 4 + 64 + 64 + 64 + 32
)

type walHeader struct {
	Version uint64
	Size    uint64
	Major   uint32
	Minor   uint32
	Kernel  [64]byte
	GPT     [64]byte
	FS      [64]byte
	MAC     [32]byte
}

type walHeaderV1 struct {
	Version uint64
	Size    uint64
	Kernel  [64]byte
	GPT     [64]byte
	FS      [64]byte
	MAC     [32]byte
}

type walHeaderV0 struct {
	Size   uint64
	Kernel [64]byte
	GPT    [64]byte
	FS     [64]byte
	MAC    [32]byte
}

type walFile interface {
	io.ReaderAt
	io.Writer
	io.WriterAt
	io.Seeker
	Sync() error
	Truncate(int64) error
	Close() error
	Stat() (fs.FileInfo, error)
	Name() string
}

type WAL struct {
	f      walFile
	header walHeader
	ranges []Range
}

// syncDirFunc flushes a directory to stable storage. It is a variable to allow tests
// to stub the implementation.
var syncDirFunc = syncDir

func walHeaderMAC(h *walHeader) [32]byte {
	var buf [8 + 8 + 4 + 4 + 64 + 64 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Version)
	binary.LittleEndian.PutUint64(buf[8:16], h.Size)
	binary.LittleEndian.PutUint32(buf[16:20], h.Major)
	binary.LittleEndian.PutUint32(buf[20:24], h.Minor)
	copy(buf[24:88], h.Kernel[:])
	copy(buf[88:152], h.GPT[:])
	copy(buf[152:], h.FS[:])
	return blake3.Sum256(buf[:])
}

func walHeaderMACV1(h *walHeaderV1) [32]byte {
	var buf [8 + 8 + 64 + 64 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Version)
	binary.LittleEndian.PutUint64(buf[8:16], h.Size)
	copy(buf[16:80], h.Kernel[:])
	copy(buf[80:144], h.GPT[:])
	copy(buf[144:], h.FS[:])
	return blake3.Sum256(buf[:])
}

func walHeaderMACV0(h *walHeaderV0) [32]byte {
	var buf [8 + 64 + 64 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Size)
	copy(buf[8:72], h.Kernel[:])
	copy(buf[72:136], h.GPT[:])
	copy(buf[136:], h.FS[:])
	return blake3.Sum256(buf[:])
}

// OpenWAL opens or creates a WAL at path for the given device identity.
// It verifies metadata on existing WALs and loads recorded ranges.
func OpenWAL(path string, id DeviceIdentity) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	w := &WAL{f: f}
	if st.Size() == 0 {
		var hdr walHeader
		hdr.Version = walVersion
		hdr.Size = id.SizeBytes
		hdr.Major = id.Major
		hdr.Minor = id.Minor
		copy(hdr.Kernel[:], []byte(id.KernelUUID))
		copy(hdr.GPT[:], []byte(id.GPTUUID))
		copy(hdr.FS[:], []byte(id.FSUUID))
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint32(buf[16:20], hdr.Major)
		binary.LittleEndian.PutUint32(buf[20:24], hdr.Minor)
		copy(buf[24:88], hdr.Kernel[:])
		copy(buf[88:152], hdr.GPT[:])
		copy(buf[152:216], hdr.FS[:])
		copy(buf[216:248], hdr.MAC[:])
		if n, err := f.Write(buf[:]); err != nil {
			f.Close()
			return nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, err
		}
		if err := syncDirFunc(filepath.Dir(path)); err != nil {
			f.Close()
			return nil, err
		}
		w.header = hdr
		return w, nil
	}

	var vbuf [8]byte
	if _, err := f.ReadAt(vbuf[:], 0); err != nil {
		f.Close()
		return nil, err
	}
	ver := binary.LittleEndian.Uint64(vbuf[:])
	if ver == walVersion && st.Size() >= walHeaderSize {
		var buf [walHeaderSize]byte
		if _, err := f.ReadAt(buf[:], 0); err != nil {
			f.Close()
			return nil, err
		}
		var hdr walHeader
		hdr.Version = binary.LittleEndian.Uint64(buf[0:8])
		hdr.Size = binary.LittleEndian.Uint64(buf[8:16])
		hdr.Major = binary.LittleEndian.Uint32(buf[16:20])
		hdr.Minor = binary.LittleEndian.Uint32(buf[20:24])
		copy(hdr.Kernel[:], buf[24:88])
		copy(hdr.GPT[:], buf[88:152])
		copy(hdr.FS[:], buf[152:216])
		copy(hdr.MAC[:], buf[216:248])
		if mac := walHeaderMAC(&hdr); mac != hdr.MAC {
			f.Close()
			return nil, fmt.Errorf("wal: header mac mismatch")
		}
		if hdr.Size != id.SizeBytes || hdr.Major != id.Major || hdr.Minor != id.Minor ||
			string(hdr.Kernel[:len(id.KernelUUID)]) != id.KernelUUID ||
			string(hdr.GPT[:len(id.GPTUUID)]) != id.GPTUUID ||
			string(hdr.FS[:len(id.FSUUID)]) != id.FSUUID {
			f.Close()
			return nil, fmt.Errorf("wal: metadata mismatch")
		}
		w.header = hdr
		off := int64(walHeaderSize)
		entry := make([]byte, 16)
		for {
			n, err := f.ReadAt(entry, off)
			if err != nil || n != 16 {
				break
			}
			start := binary.LittleEndian.Uint64(entry[0:8])
			end := binary.LittleEndian.Uint64(entry[8:16])
			w.ranges = append(w.ranges, Range{Start: start, End: end})
			off += 16
		}
		if _, err := f.Seek(off, 0); err != nil {
			f.Close()
			return nil, err
		}
		return w, nil
	}

	if ver == walVersion-1 && st.Size() >= walHeaderV1Size {
		var buf1 [walHeaderV1Size]byte
		if _, err := f.ReadAt(buf1[:], 0); err != nil {
			f.Close()
			return nil, err
		}
		var hdr1 walHeaderV1
		hdr1.Version = binary.LittleEndian.Uint64(buf1[0:8])
		hdr1.Size = binary.LittleEndian.Uint64(buf1[8:16])
		copy(hdr1.Kernel[:], buf1[16:80])
		copy(hdr1.GPT[:], buf1[80:144])
		copy(hdr1.FS[:], buf1[144:208])
		copy(hdr1.MAC[:], buf1[208:240])
		if mac := walHeaderMACV1(&hdr1); mac != hdr1.MAC {
			f.Close()
			return nil, fmt.Errorf("wal: header mac mismatch")
		}
		if hdr1.Size != id.SizeBytes ||
			string(hdr1.Kernel[:len(id.KernelUUID)]) != id.KernelUUID ||
			string(hdr1.GPT[:len(id.GPTUUID)]) != id.GPTUUID ||
			string(hdr1.FS[:len(id.FSUUID)]) != id.FSUUID {
			f.Close()
			return nil, fmt.Errorf("wal: metadata mismatch")
		}
		data := make([]byte, st.Size()-walHeaderV1Size)
		if _, err := f.ReadAt(data, walHeaderV1Size); err != nil && err != io.EOF {
			f.Close()
			return nil, err
		}
		var hdr walHeader
		hdr.Version = walVersion
		hdr.Size = hdr1.Size
		hdr.Major = id.Major
		hdr.Minor = id.Minor
		hdr.Kernel = hdr1.Kernel
		hdr.GPT = hdr1.GPT
		hdr.FS = hdr1.FS
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint32(buf[16:20], hdr.Major)
		binary.LittleEndian.PutUint32(buf[20:24], hdr.Minor)
		copy(buf[24:88], hdr.Kernel[:])
		copy(buf[88:152], hdr.GPT[:])
		copy(buf[152:216], hdr.FS[:])
		copy(buf[216:248], hdr.MAC[:])
		if n, err := f.WriteAt(buf[:], 0); err != nil {
			f.Close()
			return nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if len(data) > 0 {
			if n, err := f.WriteAt(data, walHeaderSize); err != nil {
				f.Close()
				return nil, err
			} else if n != len(data) {
				f.Close()
				return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(data))
			}
		}
		if err := f.Truncate(int64(walHeaderSize + len(data))); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, err
		}
		w.header = hdr
		w.ranges = []Range{}
		for off := 0; off+16 <= len(data); off += 16 {
			start := binary.LittleEndian.Uint64(data[off : off+8])
			end := binary.LittleEndian.Uint64(data[off+8 : off+16])
			w.ranges = append(w.ranges, Range{Start: start, End: end})
		}
		if _, err := f.Seek(int64(walHeaderSize+len(data)), 0); err != nil {
			f.Close()
			return nil, err
		}
		return w, nil
	}

	if st.Size() >= walHeaderV0Size {
		var buf0 [walHeaderV0Size]byte
		if _, err := f.ReadAt(buf0[:], 0); err != nil {
			f.Close()
			return nil, err
		}
		var hdr0 walHeaderV0
		hdr0.Size = binary.LittleEndian.Uint64(buf0[0:8])
		copy(hdr0.Kernel[:], buf0[8:72])
		copy(hdr0.GPT[:], buf0[72:136])
		copy(hdr0.FS[:], buf0[136:200])
		copy(hdr0.MAC[:], buf0[200:])
		if mac := walHeaderMACV0(&hdr0); mac != hdr0.MAC {
			f.Close()
			return nil, fmt.Errorf("wal: header mac mismatch")
		}
		if hdr0.Size != id.SizeBytes ||
			string(hdr0.Kernel[:len(id.KernelUUID)]) != id.KernelUUID ||
			string(hdr0.GPT[:len(id.GPTUUID)]) != id.GPTUUID ||
			string(hdr0.FS[:len(id.FSUUID)]) != id.FSUUID {
			f.Close()
			return nil, fmt.Errorf("wal: metadata mismatch")
		}
		data := make([]byte, st.Size()-walHeaderV0Size)
		if _, err := f.ReadAt(data, walHeaderV0Size); err != nil && err != io.EOF {
			f.Close()
			return nil, err
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
		hdr.Major = id.Major
		hdr.Minor = id.Minor
		hdr.Kernel = hdr0.Kernel
		hdr.GPT = hdr0.GPT
		hdr.FS = hdr0.FS
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint32(buf[16:20], hdr.Major)
		binary.LittleEndian.PutUint32(buf[20:24], hdr.Minor)
		copy(buf[24:88], hdr.Kernel[:])
		copy(buf[88:152], hdr.GPT[:])
		copy(buf[152:216], hdr.FS[:])
		copy(buf[216:248], hdr.MAC[:])
		if n, err := f.WriteAt(buf[:], 0); err != nil {
			f.Close()
			return nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if len(data) > 0 {
			if n, err := f.WriteAt(data, walHeaderSize); err != nil {
				f.Close()
				return nil, err
			} else if n != len(data) {
				f.Close()
				return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(data))
			}
		}
		if err := f.Truncate(int64(walHeaderSize + len(data))); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, err
		}
		w.header = hdr
		w.ranges = ranges
		if _, err := f.Seek(int64(walHeaderSize+len(data)), 0); err != nil {
			f.Close()
			return nil, err
		}
		return w, nil
	}

	f.Close()
	return nil, fmt.Errorf("wal: file too small")
}

func (w *WAL) Append(r Range) error {
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], r.Start)
	binary.LittleEndian.PutUint64(buf[8:16], r.End)
	n, err := w.f.Write(buf[:])
	if err != nil {
		return err
	}
	if n != len(buf) {
		return fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
	}
	return w.f.Sync()
}

// Ranges returns the ranges recorded in the WAL.
func (w *WAL) Ranges() []Range { return append([]Range(nil), w.ranges...) }

func (w *WAL) Close() error {
	if w.f == nil {
		return nil
	}
	if err := w.f.Sync(); err != nil {
		w.f.Close()
		return err
	}
	name := w.f.Name()
	if err := w.f.Close(); err != nil {
		return err
	}
	w.f = nil
	return syncDirFunc(filepath.Dir(name))
}

// SetSyncDirFunc overrides the directory sync implementation. It returns a restore function.
func SetSyncDirFunc(fn func(string) error) func() {
	orig := syncDirFunc
	if fn == nil {
		syncDirFunc = syncDir
	} else {
		syncDirFunc = fn
	}
	return func() { syncDirFunc = orig }
}

func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
