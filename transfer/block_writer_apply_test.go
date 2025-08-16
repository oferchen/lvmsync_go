package transfer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

func buildBlockStream(t *testing.T, verify bool, checksum ChecksumStrategy, blocks [][]byte) *bufio.Reader {
	t.Helper()
	buf := &bytes.Buffer{}
	w := bufio.NewWriter(buf)
	for i, data := range blocks {
		offset := uint64(i * len(data))
		binary.Write(w, binary.BigEndian, offset)
		binary.Write(w, binary.BigEndian, uint32(len(data)))
		if verify {
			sum := checksum.Compute(data)
			w.Write(sum)
		}
		w.Write(data)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	return bufio.NewReader(bytes.NewReader(buf.Bytes()))
}

func TestBlockWriterSyncInterval(t *testing.T) {
	cfg := &config.Config{BlockSize: 4, SyncIntervalBytes: 8}
	checksum := GetChecksumStrategy("sha256")
	tmp := t.TempDir()
	f, err := os.CreateTemp(tmp, "dest")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer f.Close()

	bw := newBlockWriter(cfg, f, nil, false, checksum, zap.NewNop(), nil)
	blocks := [][]byte{{1, 1, 1, 1}, {2, 2, 2, 2}, {3, 3, 3, 3}, {4, 4, 4, 4}}

	calls := 0
	orig := fdatasyncFile
	fdatasyncFile = func(*os.File) error {
		calls++
		return nil
	}
	defer func() { fdatasyncFile = orig }()

	reader := buildBlockStream(t, false, checksum, blocks)
	total, err := bw.write(reader)
	if err != nil {
		t.Fatalf("write blocks: %v", err)
	}
	if total != int64(len(blocks)*len(blocks[0])) {
		t.Fatalf("total bytes %d", total)
	}
	if calls != 2 {
		t.Fatalf("expected 2 fdatasync calls, got %d", calls)
	}
}

func TestBlockWriterMACVerification(t *testing.T) {
	cfg := &config.Config{BlockSize: 4}
	checksum := GetChecksumStrategy("sha256")
	tmp := t.TempDir()
	f, err := os.CreateTemp(tmp, "dest")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer f.Close()

	bw := newBlockWriter(cfg, f, nil, true, checksum, zap.NewNop(), nil)
	data := []byte{1, 2, 3, 4}
	reader := buildBlockStream(t, true, checksum, [][]byte{data})
	if _, err := bw.write(reader); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build stream with wrong checksum
	buf := &bytes.Buffer{}
	w := bufio.NewWriter(buf)
	binary.Write(w, binary.BigEndian, uint64(0))
	binary.Write(w, binary.BigEndian, uint32(len(data)))
	sum := checksum.Compute(data)
	sum[0] ^= 0xff
	w.Write(sum)
	w.Write(data)
	w.Flush()
	badReader := bufio.NewReader(bytes.NewReader(buf.Bytes()))
	if _, err := bw.write(badReader); err == nil {
		t.Fatalf("expected checksum mismatch")
	}
}
