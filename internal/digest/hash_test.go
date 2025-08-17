package digest

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeebo/blake3"
)

func TestSumReader(t *testing.T) {
	const data = "hello"
	tests := []string{SHA256, BLAKE3}
	for _, alg := range tests {
		t.Run(alg, func(t *testing.T) {
			got, err := SumReader(strings.NewReader(data), alg)
			if err != nil {
				t.Fatalf("SumReader: %v", err)
			}
			var want [32]byte
			switch alg {
			case SHA256:
				want = sha256.Sum256([]byte(data))
			case BLAKE3:
				want = blake3.Sum256([]byte(data))
			}
			if got != want {
				t.Fatalf("digest mismatch")
			}
		})
	}
}

func TestSumReaderErrors(t *testing.T) {
	t.Run("unsupported", func(t *testing.T) {
		if _, err := SumReader(strings.NewReader(""), "foo"); err == nil {
			t.Fatalf("expected error")
		}
	})
	t.Run("read", func(t *testing.T) {
		_, err := SumReader(errReader{}, SHA256)
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestSumFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := SumFile(p, BLAKE3)
	if err != nil {
		t.Fatalf("SumFile: %v", err)
	}
	want := blake3.Sum256([]byte("data"))
	if got != want {
		t.Fatalf("digest mismatch")
	}
}

func TestSumFileErrors(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Run("missing", func(t *testing.T) {
		if _, err := SumFile(filepath.Join(dir, "missing"), SHA256); err == nil {
			t.Fatalf("expected error")
		}
	})
	t.Run("unsupported", func(t *testing.T) {
		if _, err := SumFile(p, "foo"); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestSampledSumFileSmall(t *testing.T) {
	dir := t.TempDir()
	data := []byte("tiny")
	p := filepath.Join(dir, "s")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := SampledSumFile(p, SHA256)
	if err != nil {
		t.Fatalf("SampledSumFile: %v", err)
	}
	want := sha256.Sum256(data)
	if got != want {
		t.Fatalf("digest mismatch")
	}
}

func TestSampledSumFileLarge(t *testing.T) {
	dir := t.TempDir()
	first := bytes.Repeat([]byte("a"), int(sampleSize))
	middle := bytes.Repeat([]byte("c"), 10)
	last := bytes.Repeat([]byte("b"), int(sampleSize))
	data := append(append(first, middle...), last...)
	p := filepath.Join(dir, "l")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := SampledSumFile(p, BLAKE3)
	if err != nil {
		t.Fatalf("SampledSumFile: %v", err)
	}
	h := blake3.New()
	h.Write(first)
	h.Write(last)
	var want [32]byte
	copy(want[:], h.Sum(nil))
	if got != want {
		t.Fatalf("digest mismatch")
	}
}

func TestSampledSumFileUnsupported(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := SampledSumFile(p, "foo"); err == nil {
		t.Fatalf("expected error")
	}
}
