package device

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestWALIdentityMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", FSUUID: "fs"}
	w, err := OpenWAL(path, id)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	badID := DeviceIdentity{SizeBytes: 101, KernelUUID: "dev", GPTUUID: "gpt", FSUUID: "fs"}
	if _, err := OpenWAL(path, badID); err == nil {
		t.Fatalf("expected size mismatch error")
	}
	badID = DeviceIdentity{SizeBytes: 100, KernelUUID: "dev2", GPTUUID: "gpt", FSUUID: "fs"}
	if _, err := OpenWAL(path, badID); err == nil {
		t.Fatalf("expected uuid mismatch error")
	}
	badID = DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", FSUUID: "fs2"}
	if _, err := OpenWAL(path, badID); err == nil {
		t.Fatalf("expected fs uuid mismatch error")
	}
}

func TestWALRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", FSUUID: "fs"}
	w, err := OpenWAL(path, id)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: 10}); err != nil {
		t.Fatalf("append: %v", err)
	}
	w.Close()
	w, err = OpenWAL(path, id)
	if err != nil {
		t.Fatalf("reopen wal: %v", err)
	}
	rs := w.Ranges()
	if len(rs) != 1 || rs[0].Start != 0 || rs[0].End != 10 {
		t.Fatalf("unexpected ranges %#v", rs)
	}
	w.Close()
}

func TestWALVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", FSUUID: "fs"}
	w, err := OpenWAL(path, id)
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
	copy(hdr.Kernel[:], buf[16:80])
	copy(hdr.GPT[:], buf[80:144])
	copy(hdr.FS[:], buf[144:208])
	hdr.MAC = walHeaderMAC(&hdr)
	binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
	copy(buf[208:240], hdr.MAC[:])
	if _, err := f.WriteAt(buf[:], 0); err != nil {
		t.Fatalf("write header: %v", err)
	}
	f.Close()
	if _, err := OpenWAL(path, id); err == nil {
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
	copy(hdr0.Kernel[:], []byte("dev"))
	copy(hdr0.GPT[:], []byte("gpt"))
	copy(hdr0.FS[:], []byte("fs"))
	hdr0.MAC = walHeaderMACV0(&hdr0)
	var hbuf [walHeaderV0Size]byte
	binary.LittleEndian.PutUint64(hbuf[0:8], hdr0.Size)
	copy(hbuf[8:72], hdr0.Kernel[:])
	copy(hbuf[72:136], hdr0.GPT[:])
	copy(hbuf[136:200], hdr0.FS[:])
	copy(hbuf[200:232], hdr0.MAC[:])
	if _, err := f.Write(hbuf[:]); err != nil {
		t.Fatalf("write header: %v", err)
	}
	var entry [16]byte
	binary.LittleEndian.PutUint64(entry[0:8], 0)
	binary.LittleEndian.PutUint64(entry[8:16], 10)
	if _, err := f.Write(entry[:]); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	f.Close()
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", FSUUID: "fs"}
	w, err := OpenWAL(path, id)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if w.header.Version != walVersion {
		t.Fatalf("expected version %d got %d", walVersion, w.header.Version)
	}
	rs := w.Ranges()
	if len(rs) != 1 || rs[0].Start != 0 || rs[0].End != 10 {
		t.Fatalf("unexpected ranges %#v", rs)
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
