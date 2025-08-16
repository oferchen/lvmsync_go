package transfer

import (
	"os"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/device"
)

func TestProcessBlockDiscard(t *testing.T) {
	cfg := &config.Config{BlockSize: 4, Discard: true}
	f, err := os.CreateTemp(t.TempDir(), "dest")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer f.Close()
	called := false
	restore := device.SetDiscardFunc(func(_ *os.File, off, length uint64) error {
		called = true
		if off != 0 || length != 4 {
			t.Errorf("unexpected params: off=%d len=%d", off, length)
		}
		return nil
	})
	defer restore()
	data := []byte("abcd")
	if written, err := processBlock(cfg, f, nil, false, nil, 0, nil, data, 4, zap.NewNop()); err != nil || !written {
		t.Fatalf("processBlock: %v written=%v", err, written)
	}
	if !called {
		t.Fatalf("discard not called")
	}
}

func TestProcessBlockDiscardDisabled(t *testing.T) {
	cfg := &config.Config{BlockSize: 4, Discard: false}
	f, err := os.CreateTemp(t.TempDir(), "dest")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer f.Close()
	called := false
	restore := device.SetDiscardFunc(func(_ *os.File, off, length uint64) error {
		called = true
		return nil
	})
	defer restore()
	data := []byte("abcd")
	if _, err := processBlock(cfg, f, nil, false, nil, 0, nil, data, 4, zap.NewNop()); err != nil {
		t.Fatalf("processBlock: %v", err)
	}
	if called {
		t.Fatalf("discard called when disabled")
	}
}
