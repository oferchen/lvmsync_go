package common

import "io"

// WriteOnlyReadWriter wraps an io.Writer to satisfy io.ReadWriter.
type WriteOnlyReadWriter struct{ io.Writer }

// Read always returns io.EOF, making it safe to use where a read side is required.
func (WriteOnlyReadWriter) Read([]byte) (int, error) { return 0, io.EOF }
