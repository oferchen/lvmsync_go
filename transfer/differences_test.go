package transfer

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestGetDifferences(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "meta")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	chunkSize := int64(4096)

	if _, err := tmpFile.Write(make([]byte, chunkSize)); err != nil {
		t.Fatalf("failed to write header: %v", err)
	}

	writeEntry := func(origin, snap uint64) {
		buf := make([]byte, 16)
		binary.LittleEndian.PutUint64(buf[0:8], origin)
		binary.LittleEndian.PutUint64(buf[8:16], snap)
		tmpFile.Write(buf)
	}

	writeEntry(2, 1)
	writeEntry(4, 2)
	writeEntry(0, 0)
	tmpFile.Close()

	ranges, err := GetDifferences(tmpFile.Name(), chunkSize)
	if err != nil {
		t.Fatalf("GetDifferences returned error: %v", err)
	}

	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d", len(ranges))
	}

	if ranges[0].Start != 2*chunkSize || ranges[0].End != (2+1)*chunkSize-1 {
		t.Fatalf("unexpected first range: %+v", ranges[0])
	}
	if ranges[1].Start != 4*chunkSize || ranges[1].End != (4+1)*chunkSize-1 {
		t.Fatalf("unexpected second range: %+v", ranges[1])
	}
}
