//go:build linux

package device

import (
	"errors"
	"os"
	"testing"
)

func TestDiscardRangeStubAndRestore(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "discard")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer f.Close()

	stubErr := errors.New("stub discard error")
	calls := 0
	restore := SetDiscardFunc(func(got *os.File, off, length uint64) error {
		calls++
		if got != f || off != 123 || length != 456 {
			t.Errorf("unexpected args: %v %d %d", got.Name(), off, length)
		}
		return stubErr
	})

	err = DiscardRange(f, 123, 456)
	if !errors.Is(err, stubErr) {
		t.Fatalf("expected stub error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected stub to be called once, got %d", calls)
	}

	restore()

	err = DiscardRange(f, 123, 456)
	if err == nil || errors.Is(err, stubErr) {
		t.Fatalf("expected non-stub error after restore, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("stub called after restore: %d", calls)
	}
}
