// Package compress offers a Zstd codec using klauspost/compress.
package compress

import "github.com/klauspost/compress/zstd"

// Zstd implements the Codec interface.
type Zstd struct{}

// NewZstd returns a Zstd codec.
func NewZstd() Codec { return Zstd{} }

// Compress encodes data with Zstd at level 1.
func (Zstd) Compress(in []byte) ([]byte, bool, error) {
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, false, err
	}
	out := enc.EncodeAll(in, make([]byte, 0, len(in)))
	if len(out) >= len(in) {
		return in, false, nil
	}
	return out, true, nil
}

// Decompress decodes Zstd data.
func (Zstd) Decompress(in []byte) ([]byte, error) {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	return dec.DecodeAll(in, nil)
}
