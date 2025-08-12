package transport

import (
	"encoding/binary"
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
