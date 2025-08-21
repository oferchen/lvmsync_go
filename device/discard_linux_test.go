//go:build linux

package device

import (
	"errors"
	"os"
	"testing"

	"go.uber.org/zap"
)

func TestDiscardRangeStubAndRestore(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "discard")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() { _ = f.Close() }()

	stubErr := errors.New("stub discard error")
	calls := 0
	restore := SetDiscardFunc(func(got *os.File, off, length uint64, sanitize, noNewPrivs bool, _ *zap.Logger) error {
		calls++
		if got != f || off != 123 || length != 456 || sanitize || noNewPrivs {
			t.Errorf("unexpected args: %v %d %d %v %v", got.Name(), off, length, sanitize, noNewPrivs)
		}
		return stubErr
	})

	err = DiscardRange(f, 123, 456, false, false, zap.NewNop())
	if !errors.Is(err, stubErr) {
		t.Fatalf("expected stub error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected stub to be called once, got %d", calls)
	}

	restore()

	err = DiscardRange(f, 123, 456, false, false, zap.NewNop())
	if err == nil || errors.Is(err, stubErr) {
		t.Fatalf("expected non-stub error after restore, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("stub called after restore: %d", calls)
	}
}

func TestDiscardRangeNilLogger(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "discard")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := DiscardRange(f, 0, 0, false, false, nil); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}
