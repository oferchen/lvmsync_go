// Package frame provides a simple binary implementation of the Framer
// interface using little-endian encoding.
package frame

import (
	"encoding/binary"
	"fmt"
)

const headerSize = 52

// Binary implements Framer with a fixed-size header followed by payload.
type Binary struct{}

// NewBinary returns a Framer that uses binary encoding.
func NewBinary() Framer { return Binary{} }

// Encode serializes h and appends payload.
func (Binary) Encode(h Header, payload []byte) ([]byte, error) {
	buf := make([]byte, headerSize+len(payload))
	binary.LittleEndian.PutUint16(buf[0:2], h.Version)
	binary.LittleEndian.PutUint16(buf[2:4], h.Flags)
	binary.LittleEndian.PutUint64(buf[4:12], h.Offset)
	binary.LittleEndian.PutUint32(buf[12:16], h.Length)
	binary.LittleEndian.PutUint32(buf[16:20], h.CmpLen)
	copy(buf[20:52], h.Hash[:])
	copy(buf[headerSize:], payload)
	return buf, nil
}

// Decode parses a frame into its header and payload.
func (Binary) Decode(buf []byte) (Header, []byte, error) {
	if len(buf) < headerSize {
		return Header{}, nil, fmt.Errorf("short frame")
	}
	h := Header{
		Version: binary.LittleEndian.Uint16(buf[0:2]),
		Flags:   binary.LittleEndian.Uint16(buf[2:4]),
		Offset:  binary.LittleEndian.Uint64(buf[4:12]),
		Length:  binary.LittleEndian.Uint32(buf[12:16]),
		CmpLen:  binary.LittleEndian.Uint32(buf[16:20]),
	}
	copy(h.Hash[:], buf[20:52])
	return h, buf[headerSize:], nil
}
