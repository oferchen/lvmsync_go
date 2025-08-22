//go:build gofuzz
// +build gofuzz

package device

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func FuzzOpenWAL(f *testing.F) {
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path1 := filepath.Join(dir, "wal1")
		path2 := filepath.Join(dir, "wal2")
		if err := os.WriteFile(path1, data, 0o600); err != nil {
			t.Fatalf("write wal1: %v", err)
		}
		if err := os.WriteFile(path2, data, 0o600); err != nil {
			t.Fatalf("write wal2: %v", err)
		}
		w1, err1 := OpenWAL(path1, DeviceIdentity{}, zap.NewNop(), nil)
		if err1 == nil && w1 != nil {
			_ = w1.Close()
		}
		w2, err2 := OpenWAL(path2, DeviceIdentity{}, zap.NewNop(), nil)
		if err2 == nil && w2 != nil {
			_ = w2.Close()
		}
		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("nondeterministic error: %v vs %v", err1, err2)
		}
		if err1 != nil && err2 != nil && err1.Error() != err2.Error() {
			t.Fatalf("nondeterministic error messages: %v vs %v", err1, err2)
		}
	})
}
