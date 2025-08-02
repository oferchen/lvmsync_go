package zstd

import "io"

type EncoderLevel int

type Encoder struct{}

type Decoder struct{ r io.Reader }

type nopWriteCloser struct{ io.Writer }

func (n nopWriteCloser) Close() error { return nil }

func WithEncoderLevel(level EncoderLevel) func(*Encoder) { return func(*Encoder) {} }

func NewWriter(w io.Writer, opts ...func(*Encoder)) (io.WriteCloser, error) {
	return nopWriteCloser{w}, nil
}

func NewReader(r io.Reader, opts ...func(*Decoder)) (*Decoder, error) {
	return &Decoder{r: r}, nil
}

func (d *Decoder) Read(p []byte) (int, error) { return d.r.Read(p) }
func (d *Decoder) Close() error               { return nil }
