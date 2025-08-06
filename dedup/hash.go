package dedup

import (
	"hash"

	"github.com/zeebo/blake3"
)

// Hasher wraps the BLAKE3 implementation providing optional keyed mode
// and fixed 256-bit output suitable for deduplication keys.
type Hasher struct {
	h *blake3.Hasher
}

// NewHasher returns a hasher. When key is nil an unkeyed hash is used,
// otherwise BLAKE3 is initialised in keyed mode.
func NewHasher(key []byte) *Hasher {
	var h *blake3.Hasher
	if key != nil {
		var k [32]byte
		copy(k[:], key)
		var err error
		h, err = blake3.NewKeyed(k[:])
		if err != nil {
			panic(err)
		}
	} else {
		h = blake3.New()
	}
	return &Hasher{h: h}
}

// Reset resets the underlying hasher to start a new digest.
func (h *Hasher) Reset() {
	h.h.Reset()
}

// Write implements io.Writer, forwarding to the underlying hasher.
func (h *Hasher) Write(p []byte) (int, error) {
	return h.h.Write(p)
}

// Sum256 returns the 256-bit digest of the written data and resets the
// hasher ready for the next chunk.
func (h *Hasher) Sum256() [32]byte {
	var out [32]byte
	copy(out[:], h.h.Sum(nil))
	h.h.Reset()
	return out
}

// Digest returns a hash.Hash interface to allow integration with other
// components requiring the standard interface.
func (h *Hasher) Digest() hash.Hash {
	return h.h
}
