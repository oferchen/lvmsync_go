package device

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	walpkg "lvmsync_go/internal/wal"
)

func TestWALIdentityMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	w, err := OpenWAL(path, id, zap.NewNop(), nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	badID := DeviceIdentity{SizeBytes: 101, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	if _, err := OpenWAL(path, badID, zap.NewNop(), nil); err == nil {
		t.Fatalf("expected size mismatch error")
	}
	badID = DeviceIdentity{SizeBytes: 100, KernelUUID: "dev2", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	if _, err := OpenWAL(path, badID, zap.NewNop(), nil); err == nil {
		t.Fatalf("expected uuid mismatch error")
	}
	badID = DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs2", Major: 1, Minor: 2, ManifestEpoch: 1}
	if _, err := OpenWAL(path, badID, zap.NewNop(), nil); err == nil {
		t.Fatalf("expected fs uuid mismatch error")
	}
	badID = DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 3, Minor: 2, ManifestEpoch: 1}
	if _, err := OpenWAL(path, badID, zap.NewNop(), nil); err == nil {
		t.Fatalf("expected major mismatch error")
	}
	badID = DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 4, ManifestEpoch: 1}
	if _, err := OpenWAL(path, badID, zap.NewNop(), nil); err == nil {
		t.Fatalf("expected minor mismatch error")
	}
	badID = DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 2}

	badID = DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000002", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	if _, err := OpenWAL(path, badID, zap.NewNop(), nil); err == nil {
		t.Fatalf("expected mbr mismatch error")
	}
	badID = DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 2}
	if _, err := OpenWAL(path, badID, zap.NewNop(), nil); err == nil {
		t.Fatalf("expected epoch mismatch error")
	}
}

func TestWALIdentityMismatchLogging(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	w, err := OpenWAL(path, id, zap.NewNop(), nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	badID := DeviceIdentity{SizeBytes: 101, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	core, logs := observer.New(zap.ErrorLevel)
	_, err = OpenWAL(path, badID, zap.New(core), nil)
	if err == nil {
		t.Fatalf("expected error for identity mismatch")
	}
	if !errors.Is(err, ErrWALMetadataMismatch) {
		t.Fatalf("expected ErrWALMetadataMismatch, got %v", err)
	}
	if !strings.Contains(err.Error(), "precondition") {
		t.Fatalf("expected precondition error, got %v", err)
	}
	entries := logs.FilterMessage("wal metadata mismatch").All()
	if len(entries) != 1 {
		t.Fatalf("expected wal metadata mismatch log, got %v", logs.All())
	}
	fields := entries[0].ContextMap()
	if fields["header_size_bytes"].(uint64) != 100 || fields["identity_size_bytes"].(uint64) != 101 {
		t.Fatalf("unexpected size fields: %v", fields)
	}
	if fields["header_kernel_uuid"].(string) != "dev" || fields["identity_kernel_uuid"].(string) != "dev" {
		t.Fatalf("unexpected kernel uuid fields: %v", fields)
	}
}

func TestWALRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	w, err := OpenWAL(path, id, zap.NewNop(), nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: 10}); err != nil {
		t.Fatalf("append: %v", err)
	}
	w.Close()
	w, err = OpenWAL(path, id, zap.NewNop(), nil)
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
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	w, err := OpenWAL(path, id, zap.NewNop(), nil)
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
        hdr.Major = binary.LittleEndian.Uint32(buf[24:28])
        hdr.Minor = binary.LittleEndian.Uint32(buf[28:32])
        copy(hdr.Kernel[:], buf[32:96])
        copy(hdr.GPT[:], buf[96:160])
        copy(hdr.MBR[:], buf[160:164])
        copy(hdr.FS[:], buf[164:228])
        copy(hdr.Part[:], buf[228:260])
        hdr.MAC = walHeaderMAC(&hdr)
        binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
        copy(buf[260:292], hdr.MAC[:])
        if _, err := f.WriteAt(buf[:], 0); err != nil {
                t.Fatalf("write header: %v", err)
        }
	f.Close()
	if _, err := OpenWAL(path, id, zap.NewNop(), nil); err == nil {
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
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	w, err := OpenWAL(path, id, zap.NewNop(), nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if w.header.Version != walVersion {
		t.Fatalf("expected version %d got %d", walVersion, w.header.Version)
	}
	if w.header.Epoch != id.ManifestEpoch {
		t.Fatalf("expected epoch %d got %d", id.ManifestEpoch, w.header.Epoch)
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

func TestWALSyncDirCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "f", Major: 1, Minor: 2, ManifestEpoch: 1}
	stubErr := errors.New("syncdir fail")
	var calls int
	deps := walpkg.NewDepsWithSync(func(string) error {
		calls++
		return stubErr
	})
	if _, err := OpenWAL(path, id, zap.NewNop(), deps); !errors.Is(err, stubErr) {
		t.Fatalf("expected %v got %v", stubErr, err)
	}
	if calls != 1 {
		t.Fatalf("expected syncDir called once, got %d", calls)
	}
}

func TestWALSyncDirClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "f", Major: 1, Minor: 2, ManifestEpoch: 1}
	stubErr := errors.New("syncdir fail")
	var calls int
	deps := walpkg.NewDepsWithSync(func(string) error {
		calls++
		if calls > 1 {
			return stubErr
		}
		return nil
	})
	w, err := OpenWAL(path, id, zap.NewNop(), deps)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); !errors.Is(err, stubErr) {
		t.Fatalf("expected %v got %v", stubErr, err)
	}
	if calls != 2 {
		t.Fatalf("expected syncDir called twice, got %d", calls)
	}
}

func TestWALSyncDirAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "f", Major: 1, Minor: 2}
	stubErr := errors.New("syncdir fail")
	var calls int
	deps := walpkg.NewDepsWithSync(func(string) error {
		calls++
		if calls > 1 {
			return stubErr
		}
		return nil
	})
	w, err := OpenWAL(path, id, zap.NewNop(), deps)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: 1}); !errors.Is(err, stubErr) {
		t.Fatalf("expected %v got %v", stubErr, err)
	}
	if calls != 2 {
		t.Fatalf("expected syncDir called twice, got %d", calls)
	}
	w.Close()
}
