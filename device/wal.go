package device

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/zeebo/blake3"
)

type walHeader struct {
	Size   uint64
	Kernel [64]byte
	GPT    [64]byte
	FS     [64]byte
	MAC    [32]byte
}

type WAL struct {
	f      *os.File
	header walHeader
	ranges []Range
}

func walHeaderMAC(h *walHeader) [32]byte {
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
		hdr.Size = id.SizeBytes
		copy(hdr.Kernel[:], []byte(id.KernelUUID))
		copy(hdr.GPT[:], []byte(id.GPTUUID))
		copy(hdr.FS[:], []byte(id.FSUUID))
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [8 + 64 + 64 + 64 + 32]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Size)
		copy(buf[8:72], hdr.Kernel[:])
		copy(buf[72:136], hdr.GPT[:])
		copy(buf[136:200], hdr.FS[:])
		copy(buf[200:], hdr.MAC[:])
		if _, err := f.Write(buf[:]); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, err
		}
		w.header = hdr
		return w, nil
	}
	var buf [8 + 64 + 64 + 64 + 32]byte
	if _, err := f.ReadAt(buf[:], 0); err != nil {
		f.Close()
		return nil, err
	}
	var hdr walHeader
	hdr.Size = binary.LittleEndian.Uint64(buf[0:8])
	copy(hdr.Kernel[:], buf[8:72])
	copy(hdr.GPT[:], buf[72:136])
	copy(hdr.FS[:], buf[136:200])
	copy(hdr.MAC[:], buf[200:])
	if mac := walHeaderMAC(&hdr); mac != hdr.MAC {
		f.Close()
		return nil, fmt.Errorf("wal: header mac mismatch")
	}
	if hdr.Size != id.SizeBytes ||
		string(hdr.Kernel[:len(id.KernelUUID)]) != id.KernelUUID ||
		string(hdr.GPT[:len(id.GPTUUID)]) != id.GPTUUID ||
		string(hdr.FS[:len(id.FSUUID)]) != id.FSUUID {
		f.Close()
		return nil, fmt.Errorf("wal: metadata mismatch")
	}
	w.header = hdr
	off := int64(len(buf))
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
	return w, nil
}

func (w *WAL) Append(r Range) error {
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], r.Start)
	binary.LittleEndian.PutUint64(buf[8:16], r.End)
	if _, err := w.f.Write(buf[:]); err != nil {
		return err
	}
	return w.f.Sync()
}

// Ranges returns the ranges recorded in the WAL.
func (w *WAL) Ranges() []Range { return append([]Range(nil), w.ranges...) }

func (w *WAL) Close() error {
	if w.f != nil {
		return w.f.Close()
	}
	return nil
}
