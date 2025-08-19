//go:build linux
// +build linux

package transfer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"testing"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/internal/config"
)

func TestNextDataOffset(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "sparse-src")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	block := int64(4096)
	if _, err := f.Seek(block*2, 0); err != nil {
		t.Fatalf("seek: %v", err)
	}
	if _, err := f.Write([]byte("data")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if !seekHoleSupported(f) {
		t.Skip("SEEK_HOLE not supported")
	}
	off, err := nextDataOffset(f, 0)
	if err != nil {
		t.Fatalf("nextDataOffset: %v", err)
	}
	if off != block*2 {
		t.Fatalf("expected offset %d, got %d", block*2, off)
	}
}

func TestIterateBlocksSkipsSparseRegions(t *testing.T) {
	bs := 4096
	src, err := os.CreateTemp(t.TempDir(), "src")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(src.Name())
	defer src.Close()

	data := bytes.Repeat([]byte{1}, bs)
	if _, err := src.Seek(int64(bs), 0); err != nil {
		t.Fatalf("seek: %v", err)
	}
	if _, err := src.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := &config.Config{BlockSize: bs, MaxRetries: 1}
	ranges := []Range{{Start: 0}, {Start: uint64(bs)}}
	var buf bytes.Buffer
	bufOut := bufio.NewWriter(&buf)
	_, _, _, err = iterateBlocks(context.Background(), cfg, ranges, src, bufOut, nil, [2]int{-1, -1}, zap.NewNop(), &resumeTracker{})
	if err != nil {
		t.Fatalf("iterateBlocks: %v", err)
	}
	bufOut.Flush()

	expected := 16 + 16 + bs
	if buf.Len() != expected {
		t.Fatalf("expected stream size %d, got %d", expected, buf.Len())
	}

	dest, err := os.CreateTemp(t.TempDir(), "dest")
	if err != nil {
		t.Fatalf("CreateTemp dest: %v", err)
	}
	defer os.Remove(dest.Name())
	defer dest.Close()

	rd := bytes.NewReader(buf.Bytes())
	header := make([]byte, 16)
	for {
		if _, err := io.ReadFull(rd, header); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("read header: %v", err)
		}
		off := binary.BigEndian.Uint64(header[0:8])
		size := binary.BigEndian.Uint32(header[8:12])
		crc := binary.BigEndian.Uint32(header[12:16])
		var block []byte
		if size > 0 {
			block = make([]byte, size)
			if _, err := io.ReadFull(rd, block); err != nil {
				t.Fatalf("read block: %v", err)
			}
		}
		if _, err := processBlock(cfg, dest, nil, nil, false, nil, off, crc, nil, block, size, zap.NewNop(), nil); err != nil {
			t.Fatalf("processBlock: %v", err)
		}
	}

	if !seekHoleSupported(dest) {
		t.Skip("SEEK_HOLE not supported on dest")
	}
	off, err := unix.Seek(int(dest.Fd()), 0, unix.SEEK_DATA)
	if err != nil {
		t.Fatalf("seek data: %v", err)
	}
	if off != int64(bs) {
		t.Fatalf("expected first data at %d, got %d", bs, off)
	}
	buf2 := make([]byte, bs)
	if _, err := dest.ReadAt(buf2, int64(bs)); err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(buf2, data) {
		t.Fatalf("dest block mismatch")
	}
}
