//go:build linux

package device

import (
	"errors"
	"os"
	"testing"

	"go.uber.org/zap"
)

func TestDiscardRangeCustomFunc(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "discard")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() { _ = f.Close() }()

	stubErr := errors.New("stub discard error")
	calls := 0
	d := NewDiscarderWithFunc(func(got *os.File, off, length uint64, sanitize, noNewPrivs bool, _ *zap.Logger) error {
		calls++
		if got != f || off != 123 || length != 456 || sanitize || noNewPrivs {
			t.Errorf("unexpected args: %v %d %d %v %v", got.Name(), off, length, sanitize, noNewPrivs)
		}
		return stubErr
	})

	err = d.DiscardRange(f, 123, 456, false, false, zap.NewNop())
	if !errors.Is(err, stubErr) {
		t.Fatalf("expected stub error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected stub to be called once, got %d", calls)
	}

	err = NewDiscarder().DiscardRange(f, 123, 456, false, false, zap.NewNop())
	if err == nil || errors.Is(err, stubErr) {
		t.Fatalf("expected non-stub error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("stub called unexpectedly: %d", calls)
	}
}

func TestDiscardRangeNilLogger(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "discard")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() { _ = f.Close() }()
	d := NewDiscarder()
	if err := d.DiscardRange(f, 0, 0, false, false, nil); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}
