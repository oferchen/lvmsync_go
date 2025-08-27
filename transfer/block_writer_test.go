package transfer

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/config"
)

func TestBlockWriterSyncIntervalTriggers(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "dest-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer tmp.Close()

	cfg := &config.Config{BlockSize: 4, SyncIntervalBytes: 8}
	calls := 0
	deps := &Deps{FdatasyncFile: func(f *os.File) error {
		calls++
		return nil
	}}
	bw, err := newBlockWriterWithDeps(cfg, tmp, nil, false, nil, zap.NewNop(), nil, nil, deps)
	if err != nil {
		t.Fatalf("newBlockWriter: %v", err)
	}

	var buf bytes.Buffer
	hdr := make([]byte, 16)
	data1 := []byte{1, 2, 3, 4}
	binary.BigEndian.PutUint64(hdr[0:8], 0)
	binary.BigEndian.PutUint32(hdr[8:12], 4)
	binary.BigEndian.PutUint32(hdr[12:16], crc32c(data1))
	buf.Write(hdr)
	buf.Write(data1)
	data2 := []byte{5, 6, 7, 8}
	binary.BigEndian.PutUint64(hdr[0:8], 4)
	binary.BigEndian.PutUint32(hdr[8:12], 4)
	binary.BigEndian.PutUint32(hdr[12:16], crc32c(data2))
	buf.Write(hdr)
	buf.Write(data2)

	// calls incremented via deps above

	if _, err := bw.write(bufio.NewReader(&buf)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 sync, got %d", calls)
	}
}

func TestBlockWriterInvalidSyncInterval(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "dest-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer tmp.Close()

	cases := []string{"bogus", "10%"}
	for _, interval := range cases {
		cfg := &config.Config{BlockSize: 4, SyncInterval: interval}
		if _, err := newBlockWriter(cfg, tmp, nil, false, nil, zap.NewNop(), nil, nil); err == nil {
			t.Fatalf("newBlockWriter(%q) expected error", interval)
		}
	}
}
