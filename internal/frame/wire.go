// Package frame defines the on-wire representation for transferred chunks.
package frame

// Header precedes each payload and is encoded in little-endian order.
type Header struct {
	Version uint16
	Flags   uint16
	Offset  uint64
	Length  uint32
	CmpLen  uint32
	Hash    [32]byte
}

// Framer encodes and decodes frames.
type Framer interface {
	Encode(h Header, payload []byte) ([]byte, error)
	Decode(buf []byte) (Header, []byte, error)
}
