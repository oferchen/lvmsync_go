package transfer

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
	walVersion      = 1
	walHeaderV0Size = 8 + 8 + 64 + 32
	walHeaderSize   = 8 + walHeaderV0Size
)

type walHeader struct {
	Version  uint64
	Size     uint64
	Epoch    uint64
	DeviceID [64]byte
	MAC      [32]byte
}

type walHeaderV0 struct {
	Size     uint64
	Epoch    uint64
	DeviceID [64]byte
	MAC      [32]byte
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
}

// syncDirFunc flushes a directory to stable storage. It is a variable to allow tests
// to stub the implementation.
var syncDirFunc = syncDir

func walHeaderMAC(h *walHeader) [32]byte {
	var buf [8 + 8 + 8 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Version)
	binary.LittleEndian.PutUint64(buf[8:16], h.Size)
	binary.LittleEndian.PutUint64(buf[16:24], h.Epoch)
	copy(buf[24:], h.DeviceID[:])
	return blake3.Sum256(buf[:])
}

func walHeaderMACV0(h *walHeaderV0) [32]byte {
	var buf [8 + 8 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Size)
	binary.LittleEndian.PutUint64(buf[8:16], h.Epoch)
	copy(buf[16:], h.DeviceID[:])
	return blake3.Sum256(buf[:])
}

// OpenWAL opens or creates the WAL at path. It validates the metadata and
// returns any fully committed ranges. If a crash left a partially written
// entry, the WAL is truncated to the last complete record and positioned for
// further appends.
func OpenWAL(path string, size uint64, deviceID string, epoch uint64) (*WAL, []Range, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	w := &WAL{f: f}
	if st.Size() == 0 {
		w.header.Version = walVersion
		w.header.Size = size
		w.header.Epoch = epoch
		copy(w.header.DeviceID[:], []byte(deviceID))
		w.header.MAC = walHeaderMAC(&w.header)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], w.header.Version)
		binary.LittleEndian.PutUint64(buf[8:16], w.header.Size)
		binary.LittleEndian.PutUint64(buf[16:24], w.header.Epoch)
		copy(buf[24:88], w.header.DeviceID[:])
		copy(buf[88:120], w.header.MAC[:])
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
		if err := syncDirFunc(filepath.Dir(path)); err != nil {
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
		copy(hdr.DeviceID[:], buf[24:88])
		copy(hdr.MAC[:], buf[88:120])
		if mac := walHeaderMAC(&hdr); mac != hdr.MAC {
			f.Close()
			return nil, nil, fmt.Errorf("wal: header mac mismatch")
		}
		if hdr.Size != size || hdr.Epoch != epoch || string(hdr.DeviceID[:len(deviceID)]) != deviceID {
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

	if st.Size() >= walHeaderV0Size {
		var buf0 [walHeaderV0Size]byte
		if _, err := f.ReadAt(buf0[:], 0); err != nil {
			f.Close()
			return nil, nil, err
		}
		var hdr0 walHeaderV0
		hdr0.Size = binary.LittleEndian.Uint64(buf0[0:8])
		hdr0.Epoch = binary.LittleEndian.Uint64(buf0[8:16])
		copy(hdr0.DeviceID[:], buf0[16:80])
		copy(hdr0.MAC[:], buf0[80:112])
		if mac := walHeaderMACV0(&hdr0); mac != hdr0.MAC {
			f.Close()
			return nil, nil, fmt.Errorf("wal: header mac mismatch")
		}
		if hdr0.Size != size || hdr0.Epoch != epoch || string(hdr0.DeviceID[:len(deviceID)]) != deviceID {
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
		hdr.DeviceID = hdr0.DeviceID
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint64(buf[16:24], hdr.Epoch)
		copy(buf[24:88], hdr.DeviceID[:])
		copy(buf[88:120], hdr.MAC[:])
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

// Append records the range using a temporary file for crash safety. The range is
// written to `<path>.tmp`, fsynced, atomically renamed to the WAL path, and the
// directory is fsynced before returning.
func (w *WAL) Append(r Range) error {
	name := w.f.Name()
	tmpPath := name + ".tmp"
	tf, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := w.f.Seek(0, 0); err != nil {
		tf.Close()
		os.Remove(tmpPath)
		return err
	}
	st, err := w.f.Stat()
	if err != nil {
		tf.Close()
		os.Remove(tmpPath)
		return err
	}
	if _, err := io.Copy(tf, io.NewSectionReader(w.f, 0, st.Size())); err != nil {
		tf.Close()
		os.Remove(tmpPath)
		return err
	}
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], r.Start)
	binary.LittleEndian.PutUint64(buf[8:16], r.End)
	if n, err := tf.Write(buf[:]); err != nil {
		tf.Close()
		os.Remove(tmpPath)
		return err
	} else if n != len(buf) {
		tf.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
	}
	if err := tf.Sync(); err != nil {
		tf.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tf.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := w.f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, name); err != nil {
		return err
	}
	nf, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if _, err := nf.Seek(0, io.SeekEnd); err != nil {
		nf.Close()
		return err
	}
	w.f = nf
	return syncDirFunc(filepath.Dir(name))
}

// Sync flushes the WAL to stable storage.
func (w *WAL) Sync() error {
	if w.f != nil {
		return w.f.Sync()
	}
	return nil
}

// Close flushes the WAL and fsyncs its parent directory.
func (w *WAL) Close() error {
	if w.f == nil {
		return nil
	}
	// Flush file contents first.
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

func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
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
