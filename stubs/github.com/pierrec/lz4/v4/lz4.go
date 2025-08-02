package lz4

import "io"

type Writer struct{ io.Writer }

func NewWriter(w io.Writer) io.WriteCloser { return Writer{w} }

func (w Writer) Write(p []byte) (int, error) { return w.Writer.Write(p) }
func (w Writer) Close() error                { return nil }

type Reader struct{ r io.Reader }

func NewReader(r io.Reader) *Reader { return &Reader{r: r} }

func (r *Reader) Read(p []byte) (int, error) { return r.r.Read(p) }
