//go:build linux

package transfer

import (
	"bytes"
	"os"
	"testing"
	"unsafe"
)

func TestGetAlignedBlockBuffer(t *testing.T) {
	buf := getAlignedBlockBuffer(4096)
	if uintptr(unsafe.Pointer(&buf[0]))%4096 != 0 {
		t.Fatalf("buffer not aligned to 4096: %v", buf)
	}
	putAlignedBlockBuffer(buf)
}

func TestPunchHole(t *testing.T) {
	f, err := os.CreateTemp("", "hole")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	data := bytes.Repeat([]byte{1}, 4096*2)
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := punchHole(f, 4096, 4096); err != nil {
		t.Skipf("punchHole unsupported: %v", err)
	}
	out := make([]byte, 4096)
	if _, err := f.ReadAt(out, 4096); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !isAllZero(out) {
		t.Fatalf("expected zeros after punchHole")
	}
}
