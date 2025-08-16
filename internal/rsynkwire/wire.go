package rsynkwire

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

var crcTable = crc32.MakeTable(crc32.Castagnoli)

// Stream wraps an io.ReadWriter and transmits length-prefixed frames
// with a prepended CRC32C. Each Write becomes a single frame and Recv
// yields fully validated frames to callers.
type Stream struct {
	rw io.ReadWriter
}

// NewStream wraps the provided io.ReadWriter with CRC32C framing.
func NewStream(rw io.ReadWriter) *Stream { return &Stream{rw: rw} }

// Send writes a single frame to the underlying stream, prefixing the
// payload with its length and CRC32C checksum.
func (s *Stream) Send(p []byte) error {
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(p)))
	binary.BigEndian.PutUint32(hdr[4:8], crc32.Checksum(p, crcTable))
	if _, err := s.rw.Write(hdr[:]); err != nil {
		return err
	}
	if len(p) > 0 {
		if _, err := s.rw.Write(p); err != nil {
			return err
		}
	}
	return nil
}

// Recv reads the next frame from the stream, verifying the CRC32C value.
// It returns io.EOF when the underlying stream is exhausted.
func (s *Stream) Recv() ([]byte, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(s.rw, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[0:4])
	expected := binary.BigEndian.Uint32(hdr[4:8])
	buf := make([]byte, n)
	if _, err := io.ReadFull(s.rw, buf); err != nil {
		return nil, err
	}
	if crc32.Checksum(buf, crcTable) != expected {
		return nil, fmt.Errorf("crc32c mismatch")
	}
	return buf, nil
}
