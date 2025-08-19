package transfer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/internal/config"
)

func TestBlockWriterCoalescesZeroBlocks(t *testing.T) {
	cfg := &config.Config{BlockSize: 4096}
	dest, err := os.CreateTemp(t.TempDir(), "dest")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(dest.Name())
	defer dest.Close()

	var stream bytes.Buffer
	hdr := make([]byte, 16)
	binary.BigEndian.PutUint64(hdr[0:8], 0)
	binary.BigEndian.PutUint32(hdr[8:12], 0)
	binary.BigEndian.PutUint32(hdr[12:16], 0)
	stream.Write(hdr)
	binary.BigEndian.PutUint64(hdr[0:8], uint64(cfg.BlockSize))
	binary.BigEndian.PutUint32(hdr[8:12], 0)
	binary.BigEndian.PutUint32(hdr[12:16], 0)
	stream.Write(hdr)
	data := bytes.Repeat([]byte{1}, cfg.BlockSize)
	binary.BigEndian.PutUint64(hdr[0:8], uint64(cfg.BlockSize*2))
	binary.BigEndian.PutUint32(hdr[8:12], uint32(cfg.BlockSize))
	binary.BigEndian.PutUint32(hdr[12:16], crc32c(data))
	stream.Write(hdr)
	stream.Write(data)

	bw, err := newBlockWriter(cfg, dest, nil, false, nil, zap.NewNop(), nil)
	if err != nil {
		t.Fatalf("newBlockWriter: %v", err)
	}
	orig := punchHoleFunc
	var calls int
	var length int
	punchHoleFunc = func(f *os.File, off uint64, l int) error {
		calls++
		length = l
		return orig(f, off, l)
	}
	defer func() { punchHoleFunc = orig }()

	if _, err := bw.write(bufio.NewReader(bytes.NewReader(stream.Bytes()))); err != nil {
		t.Fatalf("write: %v", err)
	}
	if calls != 1 || length != cfg.BlockSize*2 {
		t.Fatalf("expected one hole of %d bytes, got calls=%d len=%d", cfg.BlockSize*2, calls, length)
	}
	if !seekHoleSupported(dest) {
		t.Skip("SEEK_HOLE not supported")
	}
	off, err := unix.Seek(int(dest.Fd()), 0, unix.SEEK_DATA)
	if err != nil {
		t.Fatalf("seek data: %v", err)
	}
	if off != int64(cfg.BlockSize*2) {
		t.Fatalf("expected data offset %d, got %d", cfg.BlockSize*2, off)
	}
}
