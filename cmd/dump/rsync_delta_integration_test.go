package dump

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
	digestpkg "lvmsync_go/internal/digest"
	"lvmsync_go/internal/rsyncserver"
	"lvmsync_go/internal/rsyncwire"
)

type countingConn struct {
	net.Conn
	n int64
}

func (c *countingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.n += int64(n)
	return n, err
}

func TestStreamToRemoteRsyncDelta(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.Delta = "rsync"

	dir := t.TempDir()
	origPath := dir + "/orig"
	snapPath := dir + "/snap"

	originData := bytes.Repeat([]byte("a"), 1024)
	snapshotData := append([]byte(nil), originData...)
	copy(snapshotData[100:110], []byte("ABCDEFGHIJ"))

	if err := os.WriteFile(origPath, originData, 0o600); err != nil {
		t.Fatalf("write origin: %v", err)
	}
	if err := os.WriteFile(snapPath, snapshotData, 0o600); err != nil {
		t.Fatalf("write snap: %v", err)
	}

	dev := &rsyncserverMemDevice{buf: make([]byte, len(originData))}
	copy(dev.buf, originData)
	srv := rsyncserver.New(dev, zap.NewNop(), nil, "", "")

	c1, c2 := net.Pipe()
	cc := &countingConn{Conn: c1}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(context.Background(), rsyncwire.NewStream(c2, maxFrame)) }()

	if err := StreamToRemote(context.Background(), cfg, cc, snapPath, origPath, digestpkg.SHA256, zap.NewNop()); err != nil {
		t.Fatalf("StreamToRemote: %v", err)
	}
	c1.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if !bytes.Equal(dev.buf, snapshotData) {
		t.Fatalf("device mismatch")
	}
	if cc.n >= int64(len(snapshotData)) {
		t.Fatalf("delta not efficient: sent %d >= %d", cc.n, len(snapshotData))
	}
}

// rsyncserverMemDevice mirrors the memDevice used in rsyncserver tests.
type rsyncserverMemDevice struct {
	buf []byte
}

func (m *rsyncserverMemDevice) WriteAt(p []byte, off int64) (int, error) {
	end := int(off) + len(p)
	if end > len(m.buf) {
		newBuf := make([]byte, end)
		copy(newBuf, m.buf)
		m.buf = newBuf
	}
	copy(m.buf[off:], p)
	return len(p), nil
}

func (m *rsyncserverMemDevice) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(m.buf)) {
		return 0, io.EOF
	}
	n := copy(p, m.buf[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (m *rsyncserverMemDevice) Size() int64 { return int64(len(m.buf)) }

func (m *rsyncserverMemDevice) Sync() error { return nil }
