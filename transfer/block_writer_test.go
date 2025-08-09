package transfer

import (
	"math"
	"testing"

	"lvmsync_go/config"
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
	res := &BlockResult{Offset: 5, Size: 4, Data: []byte("test")}
	header := make([]byte, 12+checksum.Size())
	n := prepareResultHeader(cfg, checksum, res, header)
	if n != len(header) {
		t.Fatalf("expected header length %d, got %d", len(header), n)
	}
}
