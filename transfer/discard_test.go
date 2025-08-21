package transfer

import (
	"os"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
)

func TestProcessBlockDiscard(t *testing.T) {
	cfg := &config.Config{BlockSize: 4, Discard: true}
	f, err := os.CreateTemp(t.TempDir(), "dest")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer f.Close()
	called := false
	restore := device.SetDiscardFunc(func(_ *os.File, off, length uint64, sanitize, noNewPrivs bool, _ *zap.Logger) error {
		called = true
		if off != 0 || length != 4 || sanitize || noNewPrivs {
			t.Errorf("unexpected params: off=%d len=%d sanitize=%v noNewPrivs=%v", off, length, sanitize, noNewPrivs)
		}
		return nil
	})
	defer restore()
	data := []byte("abcd")
	crc := crc32c(data)
	if written, _, err := processBlock(cfg, f, nil, nil, false, nil, 0, crc, nil, data, 4, zap.NewNop(), nil); err != nil || !written {
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
	restore := device.SetDiscardFunc(func(_ *os.File, off, length uint64, sanitize, noNewPrivs bool, _ *zap.Logger) error {
		called = true
		return nil
	})
	defer restore()
	data := []byte("abcd")
	crc := crc32c(data)
	if _, _, err := processBlock(cfg, f, nil, nil, false, nil, 0, crc, nil, data, 4, zap.NewNop(), nil); err != nil {
		t.Fatalf("processBlock: %v", err)
	}
	if called {
		t.Fatalf("discard called when disabled")
	}
}
