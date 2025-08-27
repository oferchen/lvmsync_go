package common

import (
	"bytes"
	"io"
	"testing"
)

func TestWriteOnlyReadWriter(t *testing.T) {
	var buf bytes.Buffer
	rw := WriteOnlyReadWriter{Writer: &buf}
	if _, err := rw.Write([]byte("hello")); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	b := make([]byte, 1)
	n, err := rw.Read(b)
	if n != 0 || err != io.EOF {
		t.Fatalf("Read() = %d, %v; want 0, io.EOF", n, err)
	}
	if got := buf.String(); got != "hello" {
		t.Fatalf("unexpected buffer content: %q", got)
	}
}
