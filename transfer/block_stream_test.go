package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"testing"

	"github.com/zeebo/blake3"

	"lvmsync_go/internal/config"
	digest "lvmsync_go/internal/digest"
)

func TestValidateOffsetAndSizeFunc(t *testing.T) {
	if _, _, err := validateOffsetAndSize(0, 1024); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, _, err := validateOffsetAndSize(math.MaxUint64, 1024); err == nil {
		t.Fatal("expected error for large offset")
	}
	if _, _, err := validateOffsetAndSize(0, -1); err == nil {
		t.Fatal("expected error for negative size")
	}
}

func TestPrepareResultHeader(t *testing.T) {
	cfg := &config.Config{VerifyChecksum: true, ChecksumAlgorithm: "sha256"}
	checksum := GetChecksumStrategy(cfg.ChecksumAlgorithm)
	sum := blake3.Sum256([]byte("test"))
	res := &BlockResult{Offset: 5, Size: 4, Data: []byte("test"), ChunkID: sum}
	header := make([]byte, 16+checksum.Size())
	n := prepareResultHeader(cfg, checksum, res, header)
	if n != len(header) {
		t.Fatalf("expected header length %d, got %d", len(header), n)
	}
	if binary.BigEndian.Uint32(header[12:16]) != crc32c(res.Data) {
		t.Fatalf("unexpected crc32c")
	}
}

func TestNewDigestHasherAuto(t *testing.T) {
	data := []byte("data")
	origAVX2, origAVX512, origNEON, origAESNI := digest.HasAVX2, digest.HasAVX512, digest.HasNEON, digest.HasAESNI
	digest.HasAVX2 = func() bool { return true }
	digest.HasAVX512 = func() bool { return false }
	digest.HasNEON = func() bool { return false }
	digest.HasAESNI = func() bool { return false }
	h := newDigestHasher(config.Auto)
	if _, ok := h.(*blake3.Hasher); !ok {
		t.Fatalf("expected BLAKE3 hasher")
	}
	digest.HasAVX2 = func() bool { return false }
	digest.HasAVX512 = func() bool { return false }
	digest.HasNEON = func() bool { return false }
	digest.HasAESNI = func() bool { return false }
	h = newDigestHasher(config.Auto)
	h.Write(data)
	sum := h.Sum(nil)
	exp := sha256.Sum256(data)
	if !bytes.Equal(sum, exp[:]) {
		t.Fatalf("expected SHA-256 sum, got %x", sum)
	}
	digest.HasAVX2, digest.HasAVX512, digest.HasNEON, digest.HasAESNI = origAVX2, origAVX512, origNEON, origAESNI
}
