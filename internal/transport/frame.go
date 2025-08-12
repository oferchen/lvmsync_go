package transport

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/zeebo/blake3"
)

const (
	// FrameMagic marks the start of a framed payload.
	FrameMagic uint32 = 0x4c564d53 // "LVMS" in ASCII
	// FrameVersion identifies the framing protocol version.
	FrameVersion uint8 = 1
)

const headerSize = 4 + 1 + 4 + 4 + 32 + 2 // MAGIC+VER+INDEX+LEN+BLAKE3+FLAGS

// Frame represents a framed payload.
// Layout: [MAGIC|VER|INDEX|LEN|BLAKE3|FLAGS|PAYLOAD]
// MAGIC: 4 bytes, VER:1, INDEX:4, LEN:4, BLAKE3:32, FLAGS:2
// Payload follows immediately after the header.
// Numeric fields are big-endian.
type Frame struct {
	Index   uint32
	Flags   uint16
	Payload []byte
}

// Flags for frame header.
const (
	FlagHandshake uint16 = 1 << iota
	FlagBitmap
)

// WriteFrame writes a frame to w using the provided index, flags and payload.
func WriteFrame(w io.Writer, index uint32, flags uint16, payload []byte) error {
	var hdr [headerSize]byte
	binary.BigEndian.PutUint32(hdr[0:4], FrameMagic)
	hdr[4] = FrameVersion
	binary.BigEndian.PutUint32(hdr[5:9], index)
	binary.BigEndian.PutUint32(hdr[9:13], uint32(len(payload)))
	sum := blake3.Sum256(payload)
	copy(hdr[13:45], sum[:])
	binary.BigEndian.PutUint16(hdr[45:47], flags)
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// ReadFrame reads a frame from r and verifies the checksum.
func ReadFrame(r io.Reader) (Frame, error) {
	var hdr [headerSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	if binary.BigEndian.Uint32(hdr[0:4]) != FrameMagic {
		return Frame{}, errors.New("invalid magic")
	}
	if hdr[4] != FrameVersion {
		return Frame{}, errors.New("invalid version")
	}
	index := binary.BigEndian.Uint32(hdr[5:9])
	l := binary.BigEndian.Uint32(hdr[9:13])
	wantSum := hdr[13:45]
	flags := binary.BigEndian.Uint16(hdr[45:47])
	payload := make([]byte, l)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Frame{}, err
	}
	sum := blake3.Sum256(payload)
	if !equalBytes(sum[:], wantSum) {
		return Frame{}, errors.New("checksum mismatch")
	}
	return Frame{Index: index, Flags: flags, Payload: payload}, nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ApplyBitmap merges bitmap payload into dst when the FlagBitmap is set.
func ApplyBitmap(dst []byte, f Frame) {
	if f.Flags&FlagBitmap == 0 {
		return
	}
	n := len(f.Payload)
	if len(dst) < n {
		n = len(dst)
	}
	for i := 0; i < n; i++ {
		dst[i] |= f.Payload[i]
	}
}
