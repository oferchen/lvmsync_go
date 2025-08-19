package transfer

import (
	"archive/zip"
	"bytes"
	crand "crypto/rand"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"testing"

	"go.uber.org/zap"
)

// generateJPEG returns a small pre-compressed JPEG blob.
func generateJPEG(t *testing.T) []byte {
	t.Helper()
	r := rand.New(rand.NewSource(1))
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			img.Set(x, y, color.RGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg encode failed: %v", err)
	}
	return buf.Bytes()
}

// generateZIP returns a compressed ZIP archive containing random data.
func generateZIP(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	data := make([]byte, 16*1024)
	if _, err := crand.Read(data); err != nil {
		t.Fatalf("rand read failed: %v", err)
	}
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("data")
	if err != nil {
		t.Fatalf("zip create failed: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("zip write failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close failed: %v", err)
	}
	return buf.Bytes()
}

func TestCompressChunkPreCompressedBlobs(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"jpeg", generateJPEG(t)},
		{"zip", generateZIP(t)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, algo, err := CompressChunk(tc.data, StrategyAuto, 0, 0, 1, 0.8, zap.NewNop())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if algo != "none" {
				t.Fatalf("expected none, got %s", algo)
			}
			if !bytes.Equal(out, tc.data) {
				t.Fatalf("data should be unchanged when compression is skipped")
			}
		})
	}
}

func TestCompressChunkLowEntropyCompresses(t *testing.T) {
	src := bytes.Repeat([]byte{'a'}, 64*1024)
	out, algo, err := CompressChunk(src, StrategyAuto, 0, 0, 1, 0.8, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if algo == "none" {
		t.Fatalf("expected compression, got none")
	}
	if len(out) >= len(src) {
		t.Fatalf("expected compressed output smaller than input")
	}
}
