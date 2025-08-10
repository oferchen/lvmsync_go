package transfer

import (
	"hash/maphash"
	"sync"
	"testing"
)

func TestRollingHashDedupComputeHashTypeMismatch(t *testing.T) {
	t.Cleanup(func() {
		rollingHashPool = sync.Pool{New: func() any { return new(maphash.Hash) }}
	})
	var wrong int
	rollingHashPool.Put(&wrong)
	r := &RollingHashDedup{seed: maphash.MakeSeed()}
	if _, err := r.computeHash([]byte("data")); err == nil {
		t.Fatalf("expected error")
	}
}
