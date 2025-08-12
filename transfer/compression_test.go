package transfer

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"github.com/pierrec/lz4/v4"

	compressiondetect "lvmsync_go/internal/compressiondetect"
)

func compressData(t *testing.T, data []byte, algo string, level int) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := NewCompressionWriter(&buf, algo, level, 1)
	if err != nil {
		t.Fatalf("writer for %s: %v", algo, err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write %s: %v", algo, err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close %s: %v", algo, err)
	}
	return buf.Bytes()
}

func decompressData(t *testing.T, compressed []byte, algo string) []byte {
	t.Helper()
	buf := bytes.NewBuffer(compressed)
	r, err := NewDecompressionReader(buf, algo, 1)
	if err != nil {
		t.Fatalf("reader for %s: %v", algo, err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read %s: %v", algo, err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close %s: %v", algo, err)
	}
	return out
}

func TestCompressionRoundTrip(t *testing.T) {
	data := []byte("some test data for compression")
	cases := []struct {
		c     string
		level int
	}{
		{"none", 0},
		{compressionLZ4, int(lz4.Level1)},
		{compressionZSTD, 1},
	}

	for _, tc := range cases {
		compressed := compressData(t, data, tc.c, tc.level)
		out := decompressData(t, compressed, tc.c)

		if !bytes.Equal(out, data) {
			t.Fatalf("roundtrip mismatch for %s", tc.c)
		}
	}
}

func TestLZ4CompressionLevels(t *testing.T) {
	data := []byte("some test data for compression")
	levels := []int{int(lz4.Fast), int(lz4.Level5), int(lz4.Level9)}

	for _, lvl := range levels {
		compressed := compressData(t, data, compressionLZ4, lvl)
		out := decompressData(t, compressed, compressionLZ4)

		if !bytes.Equal(out, data) {
			t.Fatalf("roundtrip mismatch for level %d", lvl)
		}
	}
}

func TestNewCompressionWriterLevel(t *testing.T) {
	tests := []struct {
		name    string
		algo    string
		level   int
		wantErr bool
	}{
		{"zstdValid", compressionZSTD, 3, false},
		{"zstdInvalid", compressionZSTD, 100, true},
		{"lz4Valid", compressionLZ4, int(lz4.Level3), false},
		{"lz4Invalid", compressionLZ4, 3, true},
	}

	for _, tt := range tests {
		_, err := NewCompressionWriter(io.Discard, tt.algo, tt.level, 1)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error", tt.name)
			}
		} else {
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tt.name, err)
			}
		}
	}
}

func TestNewCompressionWriterAutoDetects(t *testing.T) {
	compressiondetect.ResetForTest()
	algo := compressiondetect.DetectOptimalCompression()
	lvl := 1
	if algo == compressionLZ4 {
		lvl = int(lz4.Level1)
	}
	w, err := NewCompressionWriter(io.Discard, StrategyAuto, lvl, 1)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	switch algo {
	case compressionLZ4:
		if _, ok := w.(*pooledLz4Writer); !ok {
			t.Fatalf("expected lz4 writer")
		}
	case compressionZSTD:
		if _, ok := w.(*pooledZstdWriter); !ok {
			t.Fatalf("expected zstd writer")
		}
	default:
		t.Fatalf("unknown algo %s", algo)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestCompressChunkThreshold(t *testing.T) {
	rnd := make([]byte, 64*1024)
	if _, err := rand.Read(rnd); err != nil {
		t.Fatalf("rand: %v", err)
	}
	data := bytes.Repeat([]byte("a"), 64*1024)

	cases := []struct {
		name             string
		input            []byte
		expectCompressed bool
	}{
		{"compressible", data, true},
		{"random", rnd, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, algo, err := CompressChunk(tc.input, StrategyAuto, 0, 1, 0.9)
			if err != nil {
				t.Fatalf("compress chunk: %v", err)
			}
			if tc.expectCompressed {
				if algo == "none" || len(out) >= len(tc.input) {
					t.Fatalf("expected data to be compressed")
				}
			} else {
				if algo != "none" || !bytes.Equal(out, tc.input) {
					t.Fatalf("expected compression to be skipped")
				}
			}
		})
	}
}

// TestCompressChunkAutoSelects verifies that automatic compression selects the
// expected algorithm based on chunk size and CPU capabilities.
func TestCompressChunkAutoSelects(t *testing.T) {
	orig := supportsSIMD
	defer func() { supportsSIMD = orig }()
	origAVX2 := hasAVX2
	origNEON := hasNEON
	defer func() {
		hasAVX2 = origAVX2
		hasNEON = origNEON
	}()

	small := bytes.Repeat([]byte("a"), 64*1024)
	large := bytes.Repeat([]byte("a"), 300*1024)

	cases := []struct {
		name   string
		data   []byte
		simd   bool
		expect string
	}{
		{"smallSIMD", small, true, compressionLZ4},
		{"largeSIMD", large, true, compressionZSTD},
		{"largeNoSIMD", large, false, compressionLZ4},
		avx2   bool
		neon   bool
		expect string
	}{
		{"smallAvx2", small, true, false, compressionLZ4},
		{"largeAvx2", large, true, false, compressionZSTD},
		{"largeNeon", large, false, true, compressionZSTD},
		{"largeNoAccel", large, false, false, compressionLZ4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			supportsSIMD = func() bool { return tc.simd }
			hasAVX2 = func() bool { return tc.avx2 }
			hasNEON = func() bool { return tc.neon }
			_, algo, err := CompressChunk(tc.data, StrategyAuto, 0, 1, 1.0)
			if err != nil {
				t.Fatalf("compress chunk: %v", err)
			}
			if algo != tc.expect {
				t.Fatalf("expected %s, got %s", tc.expect, algo)
			}
		})
	}
}

func TestSelectAlgorithmZstdLevelClamp(t *testing.T) {
	origAVX2 := hasAVX2
	origNEON := hasNEON
	defer func() {
		hasAVX2 = origAVX2
		hasNEON = origNEON
	}()

	hasAVX2 = func() bool { return true }
	hasNEON = func() bool { return false }

	_, lvl := selectAlgorithm(300*1024, StrategyAuto, 5)
	if lvl != maxAutoZstdLv {
		t.Fatalf("expected level %d, got %d", maxAutoZstdLv, lvl)
	}
	_, lvl = selectAlgorithm(300*1024, StrategyAuto, 0)
	if lvl != defaultZstdLv {
		t.Fatalf("expected default level %d, got %d", defaultZstdLv, lvl)
	}
}

func TestSelectAlgorithm(t *testing.T) {
	orig := supportsSIMD
	defer func() { supportsSIMD = orig }()

	algo, lvl := selectAlgorithm(64*1024, StrategyAuto, 0)
	if algo != compressionLZ4 || lvl != int(lz4.Level1) {
		t.Fatalf("expected lz4 level1, got %s level %d", algo, lvl)
	}

	supportsSIMD = func() bool { return true }
	algo, lvl = selectAlgorithm(300*1024, StrategyAuto, 0)
	if algo != compressionZSTD || lvl != defaultZstdLv {
		t.Fatalf("expected zstd level %d, got %s level %d", defaultZstdLv, algo, lvl)
	}

	supportsSIMD = func() bool { return false }
	algo, lvl = selectAlgorithm(300*1024, StrategyAuto, 0)
	if algo != compressionLZ4 || lvl != int(lz4.Level1) {
		t.Fatalf("expected lz4 level1 fallback, got %s level %d", algo, lvl)
	}
}

//nolint:revive // complex reuse test
func TestLZ4WriterPoolReuse(t *testing.T) {
	data1 := []byte("first payload")
	data2 := []byte("second payload")

	var buf bytes.Buffer
	w1, err := NewCompressionWriter(&buf, compressionLZ4, int(lz4.Level3), 1)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	if _, err = w1.Write(data1); err != nil {
		t.Fatalf("write1: %v", err)
	}
	if err = w1.Close(); err != nil {
		t.Fatalf("close1: %v", err)
	}

	pw1, ok := w1.(*pooledLz4Writer)
	if !ok {
		t.Fatalf("expected *pooledLz4Writer")
	}
	writerPtr := pw1.Writer

	out1 := decompressData(t, buf.Bytes(), compressionLZ4)
	if !bytes.Equal(out1, data1) {
		t.Fatalf("mismatch1")
	}

	buf.Reset()
	w2, err := NewCompressionWriter(&buf, compressionLZ4, int(lz4.Level3), 1)
	if err != nil {
		t.Fatalf("new writer2: %v", err)
	}
	if _, err = w2.Write(data2); err != nil {
		t.Fatalf("write2: %v", err)
	}
	if err = w2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}

	pw2, ok := w2.(*pooledLz4Writer)
	if !ok {
		t.Fatalf("expected *pooledLz4Writer")
	}
	if pw2.Writer != writerPtr {
		t.Fatalf("writer was not reused")
	}

	out2 := decompressData(t, buf.Bytes(), compressionLZ4)
	if !bytes.Equal(out2, data2) {
		t.Fatalf("mismatch2")
	}
}

//nolint:revive // complex reuse test
func TestLZ4ReaderPoolReuse(t *testing.T) {
	data1 := []byte("alpha")
	data2 := []byte("beta")

	buf1 := bytes.NewBuffer(compressData(t, data1, compressionLZ4, int(lz4.Level3)))
	r1, err := NewDecompressionReader(buf1, compressionLZ4, 1)
	if err != nil {
		t.Fatalf("new reader1: %v", err)
	}
	out1, err := io.ReadAll(r1)
	if err != nil {
		t.Fatalf("read1: %v", err)
	}
	if err = r1.Close(); err != nil {
		t.Fatalf("close1: %v", err)
	}
	if !bytes.Equal(out1, data1) {
		t.Fatalf("mismatch1")
	}

	pr1, ok := r1.(*pooledLz4Reader)
	if !ok {
		t.Fatalf("expected *pooledLz4Reader")
	}
	readerPtr := pr1.Reader

	buf2 := bytes.NewBuffer(compressData(t, data2, compressionLZ4, int(lz4.Level3)))
	r2, err := NewDecompressionReader(buf2, compressionLZ4, 1)
	if err != nil {
		t.Fatalf("new reader2: %v", err)
	}
	pr2, ok := r2.(*pooledLz4Reader)
	if !ok || pr2.Reader != readerPtr {
		t.Fatalf("reader was not reused")
	}
	out2, err := io.ReadAll(r2)
	if err != nil {
		t.Fatalf("read2: %v", err)
	}
	if err = r2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}
	if !bytes.Equal(out2, data2) {
		t.Fatalf("mismatch2")
	}
}

func BenchmarkCompressChunk(b *testing.B) {
	data := bytes.Repeat([]byte("a"), 1<<20)
	for _, algo := range []string{StrategyAuto, compressionLZ4, compressionZSTD} {
		b.Run(algo, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, _, err := CompressChunk(data, algo, 0, 1, 0.9); err != nil {
					b.Fatalf("compress chunk: %v", err)
				}
			}
		})
	}
}

func BenchmarkSelectAlgorithm(b *testing.B) {
	origAVX2 := hasAVX2
	origNEON := hasNEON
	defer func() {
		hasAVX2 = origAVX2
		hasNEON = origNEON
	}()

	for _, sz := range []int{64 * 1024, 300 * 1024} {
		b.Run(fmt.Sprintf("size_%d", sz), func(b *testing.B) {
			hasAVX2 = func() bool { return true }
			hasNEON = func() bool { return false }
			for i := 0; i < b.N; i++ {
				selectAlgorithm(sz, StrategyAuto, 0)
			}
		})
	}
}
