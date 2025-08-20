package transfer

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestWALVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 100, "dev", 1, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	var buf [walHeaderSize]byte
	if _, err := f.ReadAt(buf[:], 0); err != nil {
		t.Fatalf("read header: %v", err)
	}
	var hdr walHeader
	hdr.Version = walVersion + 1
	hdr.Size = binary.LittleEndian.Uint64(buf[8:16])
	hdr.Epoch = binary.LittleEndian.Uint64(buf[16:24])
	copy(hdr.DeviceID[:], buf[24:88])
	hdr.MAC = walHeaderMAC(&hdr)
	binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
	copy(buf[88:120], hdr.MAC[:])
	if _, err := f.WriteAt(buf[:], 0); err != nil {
		t.Fatalf("write header: %v", err)
	}
	f.Close()
	if _, _, err := OpenWAL(path, 100, "dev", 1, nil); err == nil {
		t.Fatalf("expected version mismatch error")
	}
}

func TestWALUpgrade(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	var hdr0 walHeaderV0
	hdr0.Size = 100
	hdr0.Epoch = 1
	copy(hdr0.DeviceID[:], []byte("dev"))
	hdr0.MAC = walHeaderMACV0(&hdr0)
	var buf0 [walHeaderV0Size]byte
	binary.LittleEndian.PutUint64(buf0[0:8], hdr0.Size)
	binary.LittleEndian.PutUint64(buf0[8:16], hdr0.Epoch)
	copy(buf0[16:80], hdr0.DeviceID[:])
	copy(buf0[80:112], hdr0.MAC[:])
	if _, err := f.Write(buf0[:]); err != nil {
		t.Fatalf("write header: %v", err)
	}
	var entry [16]byte
	binary.LittleEndian.PutUint64(entry[0:8], 0)
	binary.LittleEndian.PutUint64(entry[8:16], 10)
	if _, err := f.Write(entry[:]); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	f.Close()
	w, ranges, err := OpenWAL(path, 100, "dev", 1, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if w.header.Version != walVersion {
		t.Fatalf("expected version %d got %d", walVersion, w.header.Version)
	}
	if len(ranges) != 1 || ranges[0].Start != 0 || ranges[0].End != 10 {
		t.Fatalf("unexpected ranges %#v", ranges)
	}
	w.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if want := int64(walHeaderSize + 16); info.Size() != want {
		t.Fatalf("expected size %d got %d", want, info.Size())
	}
}
