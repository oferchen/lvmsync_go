package transfer

import (
	"math"
	"testing"
)

func TestValidateOffsetAndSize(t *testing.T) {
	t.Run("valid bounds", func(t *testing.T) {
		if _, _, err := validateOffsetAndSize(math.MaxInt64, int(math.MaxUint32)); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("offset too large", func(t *testing.T) {
		if _, _, err := validateOffsetAndSize(uint64(math.MaxInt64)+1, 1); err == nil {
			t.Fatal("expected error for offset overflow")
		}
	})

	t.Run("block size negative", func(t *testing.T) {
		if _, _, err := validateOffsetAndSize(0, -1); err == nil {
			t.Fatal("expected error for negative block size")
		}
	})

	t.Run("block size too large", func(t *testing.T) {
		if _, _, err := validateOffsetAndSize(0, int(math.MaxUint32)+1); err == nil {
			t.Fatal("expected error for oversized block size")
		}
	})
}
