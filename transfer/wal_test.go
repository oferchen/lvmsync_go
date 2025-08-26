package transfer

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"lvmsync_go/device"
	walpkg "lvmsync_go/internal/wal"
)

func TestWALMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := device.DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	w, _, err := OpenWAL(path, id, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	w.Close()
	badID := id
	badID.SizeBytes = 101
	if _, _, err := OpenWAL(path, badID, nil); err == nil {
		t.Fatalf("expected size mismatch error")
	}
	badID = id
	badID.FSUUID = "fs2"
	if _, _, err := OpenWAL(path, badID, nil); err == nil {
		t.Fatalf("expected device mismatch error")
	}
	badID = id
	badID.ManifestEpoch = 2
	if _, _, err := OpenWAL(path, badID, nil); err == nil {
		t.Fatalf("expected epoch mismatch error")
	}
	badID = id
	badID.KernelUUID = "dev2"
	if _, _, err := OpenWAL(path, badID, nil); err == nil {
		t.Fatalf("expected kernel uuid mismatch error")
	}
	badID = id
	badID.GPTUUID = "gpt2"
	if _, _, err := OpenWAL(path, badID, nil); err == nil {
		t.Fatalf("expected gpt uuid mismatch error")
	}
	badID = id
	badID.MBRSignature = "00000002"
	if _, _, err := OpenWAL(path, badID, nil); err == nil {
		t.Fatalf("expected mbr signature mismatch error")
	}
	badID = id
	badID.Major = 3
	if _, _, err := OpenWAL(path, badID, nil); err == nil {
		t.Fatalf("expected major mismatch error")
	}
	badID = id
	badID.Minor = 4
	if _, _, err := OpenWAL(path, badID, nil); err == nil {
		t.Fatalf("expected minor mismatch error")
	}
}

func TestWALRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := device.DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	w, _, err := OpenWAL(path, id, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: 10}); err != nil {
		t.Fatalf("append: %v", err)
	}
	w.Close()
	w, ranges, err := OpenWAL(path, id, nil)
	if err != nil {
		t.Fatalf("reopen wal: %v", err)
	}
	if len(ranges) != 1 || ranges[0].Start != 0 || ranges[0].End != 10 {
		t.Fatalf("unexpected ranges %#v", ranges)
	}
	w.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat wal: %v", err)
	}
}

func TestWALTruncatedHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := device.DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	w, _, err := OpenWAL(path, id, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := os.Truncate(path, 10); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, _, err := OpenWAL(path, id, nil); err == nil {
		t.Fatalf("expected truncated header error")
	}
}

func TestWALPartialWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := device.DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	w, _, err := OpenWAL(path, id, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	// Append half of an entry to simulate a crash during write.
	if _, err := f.Seek(walHeaderSize, 0); err != nil {
		t.Fatalf("seek: %v", err)
	}
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], 5)
	if _, err := f.Write(buf[:]); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	w2, ranges, err := OpenWAL(path, id, nil)
	if err != nil {
		t.Fatalf("reopen wal: %v", err)
	}
	if len(ranges) != 0 {
		t.Fatalf("expected empty ranges, got %#v", ranges)
	}
	w2.Close()
	// Verify the partial entry was truncated.
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if want := int64(walHeaderSize); st.Size() != want {
		t.Fatalf("expected size %d, got %d", want, st.Size())
	}
}

func TestWALSyncDirAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	stubErr := errors.New("syncdir fail")
	var calls int
	deps := walpkg.NewDepsWithSync(func(string) error {
		calls++
		if calls > 1 {
			return stubErr
		}
		return nil
	})
	id := device.DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt", MBRSignature: "00000001", FSUUID: "fs", Major: 1, Minor: 2, ManifestEpoch: 1}
	w, _, err := OpenWAL(path, id, deps)
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
