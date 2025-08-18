package transfer

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/zeebo/blake3"
)

type walHeader struct {
	Size     uint64
	Epoch    uint64
	DeviceID [64]byte
	MAC      [32]byte
}

type WAL struct {
	f      *os.File
	header walHeader
}

func walHeaderMAC(h *walHeader) [32]byte {
	var buf [8 + 8 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Size)
	binary.LittleEndian.PutUint64(buf[8:16], h.Epoch)
	copy(buf[16:], h.DeviceID[:])
	return blake3.Sum256(buf[:])
}

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
		copy(w.header.DeviceID[:], []byte(deviceID))
		w.header.Size = size
		w.header.Epoch = epoch
		w.header.MAC = walHeaderMAC(&w.header)
		var buf [8 + 8 + 64 + 32]byte
		binary.LittleEndian.PutUint64(buf[0:8], w.header.Size)
		binary.LittleEndian.PutUint64(buf[8:16], w.header.Epoch)
		copy(buf[16:80], w.header.DeviceID[:])
		copy(buf[80:], w.header.MAC[:])
		if _, err := f.WriteAt(buf[:], 0); err != nil {
			f.Close()
			return nil, nil, err
		}
		if _, err := f.Seek(int64(len(buf)), 0); err != nil {
			f.Close()
			return nil, nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, nil, err
		}
		return w, nil, nil
	}
	var hdr walHeader
	var buf [8 + 8 + 64 + 32]byte
	if _, err := f.ReadAt(buf[:], 0); err != nil {
		f.Close()
		return nil, nil, err
	}
	hdr.Size = binary.LittleEndian.Uint64(buf[0:8])
	hdr.Epoch = binary.LittleEndian.Uint64(buf[8:16])
	copy(hdr.DeviceID[:], buf[16:80])
	copy(hdr.MAC[:], buf[80:])
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
	off := int64(len(buf))
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
	return w, ranges, nil
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

func (w *WAL) Sync() error {
	if w.f != nil {
		return w.f.Sync()
	}
	return nil
}

func (w *WAL) Close() error {
	if w.f != nil {
		return w.f.Close()
	}
	return nil
}
