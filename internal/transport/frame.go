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
=======
	"fmt"
	"io"
)

// Frame flags.
const (
	// FlagHasHash indicates the frame includes a valid hash field.
	FlagHasHash uint16 = 1 << iota
	// FlagZero denotes a chunk of zero bytes with no payload.
	FlagZero
)

const (
	indexSize  = 8
	flagSize   = 2
	hashSize   = 32
	lengthSize = 4
	headerSize = indexSize + flagSize + hashSize + lengthSize
)

// EncodeFrame writes a frame to w using the generic layout
// index|flags|hash|length|payload.
func EncodeFrame(w io.Writer, index uint64, flags uint16, hash []byte, payload []byte) error {
	var header [headerSize]byte
	binary.BigEndian.PutUint64(header[0:8], index)
	binary.BigEndian.PutUint16(header[8:10], flags)
	if len(hash) > hashSize {
		return fmt.Errorf("hash too large: %d", len(hash))
	}
	copy(header[10:42], hash)
	binary.BigEndian.PutUint32(header[42:46], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if len(payload) == 0 {
		return nil
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("write frame payload: %w", err)
	}
	return nil
}

// DecodeFrame reads a frame from r using the generic layout
// index|flags|hash|length|payload.
func DecodeFrame(r io.Reader) (index uint64, flags uint16, hash []byte, payload []byte, err error) {
	var header [headerSize]byte
	if _, err = io.ReadFull(r, header[:]); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err = io.EOF
		} else {
			err = fmt.Errorf("read frame header: %w", err)
		}
		return
	}
	index = binary.BigEndian.Uint64(header[0:8])
	flags = binary.BigEndian.Uint16(header[8:10])
	if flags&FlagHasHash != 0 {
		hash = make([]byte, hashSize)
		copy(hash, header[10:42])
	}
	size := binary.BigEndian.Uint32(header[42:46])
	if size > 0 {
		payload = make([]byte, size)
		if _, err = io.ReadFull(r, payload); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				err = io.EOF
			} else {
				err = fmt.Errorf("read frame payload: %w", err)
			}
			return
		}
	}
	return
}

// Frame represents a decoded frame for convenience in tests.
type Frame struct {
	Index   uint64
	Flags   uint16
	Hash    []byte
	Payload []byte
}

// WriteTo encodes the frame to w.
func (f *Frame) WriteTo(w io.Writer) error {
	return EncodeFrame(w, f.Index, f.Flags, f.Hash, f.Payload)
}

// ReadFrame reads a frame from r into f.
func (f *Frame) ReadFrame(r io.Reader) error {
	idx, fl, h, p, err := DecodeFrame(r)
	if err != nil {
		return err
	}
	f.Index, f.Flags, f.Hash, f.Payload = idx, fl, h, p
	return nil
}
