package manifest

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func FuzzOpenCorrupt(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, HeaderSize-1))
	garbled := make([]byte, HeaderSize)
	binary.LittleEndian.PutUint32(garbled[0:4], Version)
	// leave rest zero, so MAC mismatches
	f.Add(garbled)
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "fuzz.man")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, err := Open(path)
		if err == nil {
			t.Fatalf("Open succeeded on corrupt input of length %d", len(data))
		}
		_, err2 := Open(path)
		if err2 == nil || err2.Error() != err.Error() {
			t.Fatalf("error not deterministic: %v vs %v", err, err2)
		}
	})
}
