package transfer

import (
	"bytes"
	"sync"

	"github.com/zeebo/blake3"
)

// zeroBufCache stores zero-filled buffers keyed by their size to avoid repeated
// allocations when comparing blocks with memcmp.
var zeroBufCache sync.Map // map[int][]byte

// zeroHashCache stores BLAKE3 hashes of zero-filled buffers keyed by size.
var zeroHashCache sync.Map // map[int][32]byte

func zeroBuf(size int) []byte {
	if v, ok := zeroBufCache.Load(size); ok {
		return v.([]byte)
	}
	buf := make([]byte, size)
	zeroBufCache.Store(size, buf)
	return buf
}

func zeroHash(size int) [32]byte {
	if v, ok := zeroHashCache.Load(size); ok {
		return v.([32]byte)
	}
	sum := blake3.Sum256(zeroBuf(size))
	zeroHashCache.Store(size, sum)
	return sum
}

// isAllZero returns true if the provided slice consists entirely of zero bytes
// using memcmp semantics via bytes.Equal.
func isAllZero(b []byte) bool {
	return bytes.Equal(b, zeroBuf(len(b)))
}
