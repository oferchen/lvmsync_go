// Package compress offers an LZ4 codec using pierrec/lz4.
package compress

import (
	"bytes"
	"io"

	"github.com/pierrec/lz4/v4"
)

// LZ4 implements the Codec interface.
type LZ4 struct{}

// NewLZ4 returns an LZ4 codec.
func NewLZ4() Codec { return LZ4{} }

// Compress encodes data with LZ4 using the framed format.
func (LZ4) Compress(in []byte) ([]byte, bool, error) {
	var buf bytes.Buffer
	w := lz4.NewWriter(&buf)
	if _, err := w.Write(in); err != nil {
		return nil, false, err
	}
	if err := w.Close(); err != nil {
		return nil, false, err
	}
	out := buf.Bytes()
	if len(out) >= len(in) {
		return in, false, nil
	}
	return out, true, nil
}

// Decompress decodes LZ4 framed data.
func (LZ4) Decompress(in []byte) ([]byte, error) {
	r := lz4.NewReader(bytes.NewReader(in))
	return io.ReadAll(r)
}
