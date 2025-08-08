package transfer

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
)

func TestGetDifferences(t *testing.T) {
	tmpFile := newTempFile(t, "meta")
	defer os.Remove(tmpFile.Name())

	chunkSize := int64(4096)

	if _, err := tmpFile.Write(make([]byte, chunkSize)); err != nil {
		t.Fatalf("failed to write header: %v", err)
	}

	writeEntry := func(origin, snap uint64) {
		buf := make([]byte, 16)
		binary.LittleEndian.PutUint64(buf[0:8], origin)
		binary.LittleEndian.PutUint64(buf[8:16], snap)
		if _, err := tmpFile.Write(buf); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}

	writeEntry(2, 1)
	writeEntry(4, 2)
	writeEntry(0, 0)
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

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

func TestGetDifferencesOverflow(t *testing.T) {
	tmpFile := newTempFile(t, "meta_overflow")
	defer os.Remove(tmpFile.Name())

	chunkSize := int64(4096)

	if _, err := tmpFile.Write(make([]byte, chunkSize)); err != nil {
		t.Fatalf("failed to write header: %v", err)
	}

	buf := make([]byte, 16)
	origin := uint64(math.MaxInt64)/uint64(chunkSize) + 1
	binary.LittleEndian.PutUint64(buf[0:8], origin)
	binary.LittleEndian.PutUint64(buf[8:16], 1)
	if _, err := tmpFile.Write(buf); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := GetDifferences(tmpFile.Name(), chunkSize); err == nil {
		t.Fatalf("expected overflow error")
	}
}

func TestGetDifferencesInvalidChunkSize(t *testing.T) {
	tmpFile := newTempFile(t, "meta_invalid")
	defer os.Remove(tmpFile.Name())
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	for _, cs := range []int64{0, -1} {
		if _, err := GetDifferences(tmpFile.Name(), cs); err == nil {
			t.Fatalf("expected error for chunk size %d", cs)
		}
	}
}
