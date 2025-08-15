package transfer

import (
	"os"
	"testing"
)

func tmpFile(tb testing.TB) *os.File {
	tb.Helper()
	f, err := os.CreateTemp(tb.TempDir(), "readblock")
	if err != nil {
		tb.Fatalf("CreateTemp: %v", err)
	}
	return f
}

// TestReadBlockIntoNoAlloc ensures ReadBlockInto does not allocate.
func TestReadBlockIntoNoAlloc(t *testing.T) {
	f := tmpFile(t)
	defer f.Close()
	data := []byte("hello world")
	if _, err := f.WriteAt(data, 0); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(data))
	allocs := testing.AllocsPerRun(100, func() {
		if err := ReadBlockInto(f, 0, buf); err != nil {
			t.Fatalf("ReadBlockInto: %v", err)
		}
	})
	if allocs != 0 {
		t.Fatalf("expected zero allocations, got %f", allocs)
	}
}

func BenchmarkReadBlockInto(b *testing.B) {
	f := tmpFile(b)
	defer f.Close()
	data := make([]byte, 4096)
	if _, err := f.WriteAt(data, 0); err != nil {
		b.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(data))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := ReadBlockInto(f, 0, buf); err != nil {
			b.Fatalf("ReadBlockInto: %v", err)
		}
	}
}

func BenchmarkReadBlock(b *testing.B) {
	f := tmpFile(b)
	defer f.Close()
	data := make([]byte, 4096)
	if _, err := f.WriteAt(data, 0); err != nil {
		b.Fatalf("write: %v", err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ReadBlock(f, 0, len(data)); err != nil {
			b.Fatalf("ReadBlock: %v", err)
		}
	}
}
