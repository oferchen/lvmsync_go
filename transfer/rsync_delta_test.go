package transfer

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	digestpkg "lvmsync_go/internal/digest"
	"lvmsync_go/internal/rsyncserver"
	"lvmsync_go/internal/rsyncwire"
	manifestpkg "lvmsync_go/manifest"
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

func (m *rsyncserverMemDevice) Identity(context.Context) (device.DeviceIdentity, error) {
	return device.DeviceIdentity{SizeBytes: uint64(len(m.buf))}, nil
}

type rdMockDevice struct {
	path      string
	size      uint64
	blockSize uint64
}

func (m *rdMockDevice) Path() string      { return m.path }
func (m *rdMockDevice) SizeBytes() uint64 { return m.size }
func (m *rdMockDevice) BlockSize() uint64 { return m.blockSize }
func (m *rdMockDevice) Snapshot(ctx context.Context, snapshotSize string) (device.Device, error) {
	return m, nil
}
func (m *rdMockDevice) Cleanup(ctx context.Context) error { return nil }
func (m *rdMockDevice) Close() error                      { return nil }
func (m *rdMockDevice) Identity(context.Context) (device.DeviceIdentity, error) {
	return device.DeviceIdentity{}, nil
}
func (m *rdMockDevice) AppendWAL(device.Range) error              { return nil }
func (m *rdMockDevice) RecoverWAL(func(device.Range) error) error { return nil }

func rdBuildManifest(t *testing.T, devPath, manPath string, size uint64, cdcMin, cdcAvg, cdcMax uint32) {
	t.Helper()
	detect := func(ctx context.Context, path string, logger *zap.Logger) (device.Device, error) {
		return &rdMockDevice{path: devPath, size: size, blockSize: 4}, nil
	}
	info := device.NewInfoWithDeps(func(context.Context, string) (string, error) { return "uuid", nil }, nil, nil, nil, nil)
	if err := manifestpkg.Rebuild(context.Background(), devPath, manPath, zap.NewNop(), 0, false, cdcMin, cdcAvg, cdcMax, 0, manifestpkg.WithDetectDevice(detect), manifestpkg.WithDeviceInfo(info)); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
}

func TestDumpChangesRsyncDelta(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.Delta = "rsync"
	cfg.Compress = "none"
	cfg.ChecksumAlgorithm = digestpkg.SHA256

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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c1, c2 := net.Pipe()
	cc := &countingConn{Conn: c1}
	var wg sync.WaitGroup
	serverReady := make(chan struct{})
	var srvErr, clientErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		close(serverReady)
		srvErr = srv.Handle(ctx, rsyncwire.NewStream(c2, rsyncMaxFrame))
	}()

	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer c1.Close()
		<-serverReady
		clientErr = tr.DumpChangesSequential(ctx, cfg, snapPath, origPath, cc)
	}()

	wg.Wait()
	if err := ctx.Err(); err != nil {
		t.Fatalf("context: %v", err)
	}
	if clientErr != nil {
		t.Fatalf("DumpChangesSequential: %v", clientErr)
	}
	if srvErr != nil {
		t.Fatalf("Handle: %v", srvErr)
	}
	if !bytes.Equal(dev.buf, snapshotData) {
		t.Fatalf("device mismatch")
	}
	if cc.n >= int64(len(snapshotData)) {
		t.Fatalf("delta not efficient: sent %d >= %d", cc.n, len(snapshotData))
	}
}

func TestRsyncDeltaCDCShift(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.Delta = "rsync"
	cfg.Compress = "none"
	cfg.ChecksumAlgorithm = digestpkg.SHA256
	cfg.CDCMin = 64
	cfg.CDCAvg = 64
	cfg.CDCMax = 128

	dir := t.TempDir()
	origPath := dir + "/orig"
	snapPath := dir + "/snap"
	manPath := dir + "/man"

	originData := bytes.Repeat([]byte("a"), 512)
	snapshotData := append([]byte("abcdefghij"), originData[:len(originData)-10]...)
	if err := os.WriteFile(origPath, originData, 0o600); err != nil {
		t.Fatalf("write origin: %v", err)
	}
	if err := os.WriteFile(snapPath, snapshotData, 0o600); err != nil {
		t.Fatalf("write snap: %v", err)
	}
	rdBuildManifest(t, origPath, manPath, uint64(len(originData)), 64, 64, 128)
	cfg.ManifestPath = manPath

	dev := &rsyncserverMemDevice{buf: make([]byte, len(originData))}
	copy(dev.buf, originData)
	srv := rsyncserver.New(dev, zap.NewNop(), nil, "", "")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c1, c2 := net.Pipe()
	cc := &countingConn{Conn: c1}
	var wg sync.WaitGroup
	serverReady := make(chan struct{})
	var srvErr, clientErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		close(serverReady)
		srvErr = srv.Handle(ctx, rsyncwire.NewStream(c2, rsyncMaxFrame))
	}()

	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer c1.Close()
		<-serverReady
		clientErr = tr.DumpChangesSequential(ctx, cfg, snapPath, origPath, cc)
	}()

	wg.Wait()
	if err := ctx.Err(); err != nil {
		t.Fatalf("context: %v", err)
	}
	if clientErr != nil {
		t.Fatalf("DumpChangesSequential: %v", clientErr)
	}
	if srvErr != nil {
		t.Fatalf("Handle: %v", srvErr)
	}
	if !bytes.Equal(dev.buf, snapshotData) {
		t.Fatalf("device mismatch")
	}
	if cc.n >= int64(len(snapshotData)) {
		t.Fatalf("delta not efficient: sent %d >= %d", cc.n, len(snapshotData))
	}
}

func TestRsyncDeltaCDCMutate(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.Delta = "rsync"
	cfg.Compress = "none"
	cfg.ChecksumAlgorithm = digestpkg.SHA256
	cfg.CDCMin = 64
	cfg.CDCAvg = 64
	cfg.CDCMax = 128

	dir := t.TempDir()
	origPath := dir + "/orig"
	snapPath := dir + "/snap"
	manPath := dir + "/man"

	originData := bytes.Repeat([]byte("a"), 512)
	snapshotData := append([]byte(nil), originData...)
	copy(snapshotData[128:138], []byte("ABCDEFGHIJ"))
	if err := os.WriteFile(origPath, originData, 0o600); err != nil {
		t.Fatalf("write origin: %v", err)
	}
	if err := os.WriteFile(snapPath, snapshotData, 0o600); err != nil {
		t.Fatalf("write snap: %v", err)
	}
	rdBuildManifest(t, origPath, manPath, uint64(len(originData)), 64, 64, 128)
	cfg.ManifestPath = manPath

	dev := &rsyncserverMemDevice{buf: make([]byte, len(originData))}
	copy(dev.buf, originData)
	srv := rsyncserver.New(dev, zap.NewNop(), nil, "", "")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c1, c2 := net.Pipe()
	cc := &countingConn{Conn: c1}
	var wg sync.WaitGroup
	serverReady := make(chan struct{})
	var srvErr, clientErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		close(serverReady)
		srvErr = srv.Handle(ctx, rsyncwire.NewStream(c2, rsyncMaxFrame))
	}()

	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer c1.Close()
		<-serverReady
		clientErr = tr.DumpChangesSequential(ctx, cfg, snapPath, origPath, cc)
	}()

	wg.Wait()
	if err := ctx.Err(); err != nil {
		t.Fatalf("context: %v", err)
	}
	if clientErr != nil {
		t.Fatalf("DumpChangesSequential: %v", clientErr)
	}
	if srvErr != nil {
		t.Fatalf("Handle: %v", srvErr)
	}
	if !bytes.Equal(dev.buf, snapshotData) {
		t.Fatalf("device mismatch")
	}
	if cc.n >= int64(len(snapshotData))/2 {
		t.Fatalf("delta not efficient: sent %d >= %d", cc.n, len(snapshotData)/2)
	}
}

func TestDumpChangesDeltaDisabled(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.Compress = "none"
	cfg.ChecksumAlgorithm = digestpkg.SHA256
	blockSize := int64(1024)
	cfg.BlockSize = int(blockSize)
	changed := []int{0}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	var buf bytes.Buffer
	if err := tr.DumpChangesSequential(context.Background(), cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesSequential: %v", err)
	}
	if b := buf.Bytes(); len(b) > 0 && b[0] == 'S' {
		t.Fatalf("unexpected rsync frame")
	}
}
