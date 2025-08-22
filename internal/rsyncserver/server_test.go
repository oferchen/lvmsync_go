package rsyncserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/device"
	"lvmsync_go/internal/digest"
	"lvmsync_go/internal/rsyncwire"
	"lvmsync_go/internal/signaturecache"
)

const maxFrame = 1 << 20

type memDevice struct {
	buf   []byte
	sync  bool
	short bool
	id    device.DeviceIdentity
}

func (m *memDevice) WriteAt(p []byte, off int64) (int, error) {
	end := int(off) + len(p)
	if end > len(m.buf) {
		newBuf := make([]byte, end)
		copy(newBuf, m.buf)
		m.buf = newBuf
	}
	if m.short {
		copy(m.buf[off:], p[:len(p)-1])
		return len(p) - 1, nil
	}
	copy(m.buf[off:], p)
	return len(p), nil
}

func (m *memDevice) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(m.buf)) {
		return 0, io.EOF
	}
	n := copy(p, m.buf[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (m *memDevice) Size() int64 { return int64(len(m.buf)) }

func (m *memDevice) Sync() error { m.sync = true; return nil }

func (m *memDevice) Identity(context.Context) (device.DeviceIdentity, error) {
	if (m.id != device.DeviceIdentity{}) {
		return m.id, nil
	}
	return device.DeviceIdentity{SizeBytes: uint64(len(m.buf))}, nil
}

func newServer(t *testing.T, dev *memDevice, data []byte) *Server {
	t.Helper()
	srv, err := New(dev, zap.NewNop(), nil, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	exp, err := digest.SumReader(bytes.NewReader(data), digest.SHA256)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	srv.alg = digest.SHA256
	srv.expect = exp
	return srv
}

func sendIdentity(t *testing.T, ctx context.Context, cl *rsyncwire.Client, id device.DeviceIdentity) {
	t.Helper()
	if err := cl.SendIdentity(ctx, id); err != nil {
		t.Fatalf("SendIdentity: %v", err)
	}
}

func sendDelta(t *testing.T, ctx context.Context, cl *rsyncwire.Client, off int64, data []byte) {
	t.Helper()
	if err := cl.SendDelta(ctx, off, data); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
}

func sendDigest(t *testing.T, ctx context.Context, cl *rsyncwire.Client, alg string, sum [32]byte) {
	t.Helper()
	if err := cl.SendDigest(ctx, alg, sum); err != nil {
		t.Fatalf("SendDigest: %v", err)
	}
}

func streamSend(t *testing.T, ctx context.Context, stream *rsyncwire.Stream, b []byte) {
	t.Helper()
	if err := stream.Send(ctx, b); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func waitHandle(t *testing.T, errCh <-chan error) error {
	t.Helper()
	select {
	case err := <-errCh:
		return err
	case <-time.After(time.Second):
		t.Fatalf("Handle did not return")
		return nil
	}
}

func TestNewNilLogger(t *testing.T) {
	if _, err := New(nil, nil, nil, "", ""); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}

func TestHandleApplyDelta(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	data := []byte("hi")
	id := device.DeviceIdentity{
		SizeBytes:    uint64(len(data)),
		KernelUUID:   "kernel",
		GPTUUID:      "gpt",
		MBRSignature: "mbr",
		FSUUID:       "fs",
	}
	dev := &memDevice{buf: make([]byte, len(data)), id: id}
	srv := newServer(t, dev, data)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	cl := rsyncwire.NewClient(rsyncwire.NewStream(c1, maxFrame))
	sendIdentity(t, ctx, cl, id)
	sendDelta(t, ctx, cl, 0, data)
	c1.Close()
	if err := waitHandle(t, errCh); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	cancel()
	if string(dev.buf) != string(data) {
		t.Fatalf("device %q want %q", dev.buf, data)
	}
	if !dev.sync {
		t.Fatalf("expected sync")
	}
}

func TestHandleShortWrite(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{buf: make([]byte, 2), short: true}
	id := device.DeviceIdentity{
		SizeBytes:    uint64(len(dev.buf)),
		KernelUUID:   "kernel",
		GPTUUID:      "gpt",
		MBRSignature: "mbr",
		FSUUID:       "fs",
	}
	dev.id = id
	srv := newServer(t, dev, []byte("hi"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	cl := rsyncwire.NewClient(rsyncwire.NewStream(c1, maxFrame))
	sendIdentity(t, ctx, cl, id)
	sendDelta(t, ctx, cl, 0, []byte("hi"))
	c1.Close()
	if err := waitHandle(t, errCh); err == nil || !strings.Contains(err.Error(), "short write") {
		t.Fatalf("expected short write error, got %v", err)
	}
	cancel()
}

func TestHandleRejectsOversizedSignatures(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{buf: make([]byte, 1)}
	srv, err := New(dev, zap.NewNop(), nil, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	var buf bytes.Buffer
	buf.WriteByte('S')
	binary.Write(&buf, binary.LittleEndian, int32(2))
	binary.Write(&buf, binary.LittleEndian, int32(1))
	binary.Write(&buf, binary.LittleEndian, int32(1))
	binary.Write(&buf, binary.LittleEndian, int32(0))
	binary.Write(&buf, binary.LittleEndian, int32(0))
	buf.WriteByte(0)
	binary.Write(&buf, binary.LittleEndian, int32(0))
	buf.WriteByte(0)
	streamSend(t, ctx, rsyncwire.NewStream(c1, maxFrame), buf.Bytes())
	c1.Close()
	if err := waitHandle(t, errCh); err == nil || !strings.Contains(err.Error(), "checksum count") {
		t.Fatalf("expected checksum count error, got %v", err)
	}
	cancel()
}

func TestHandleCacheHit(t *testing.T) {
	dir := t.TempDir()
	cache := signaturecache.New(dir, time.Hour, 10)
	data := []byte("hello")
	dev := &memDevice{buf: append([]byte{}, data...)}
	dgst, err := digest.SumReader(bytes.NewReader(data), digest.SHA256)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if err := cache.Put("vg", "lv", int64(len(data)), dgst[:]); err != nil {
		t.Fatalf("Put: %v", err)
	}
	srv, err := New(dev, zap.NewNop(), cache, "vg", "lv")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	var buf bytes.Buffer
	buf.WriteByte('G')
	buf.WriteByte(byte(len(digest.SHA256)))
	buf.WriteString(digest.SHA256)
	buf.Write(dgst[:])
	streamSend(t, ctx, rsyncwire.NewStream(c1, maxFrame), buf.Bytes())
	c1.Close()
	if err := waitHandle(t, errCh); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	cancel()
	if dev.sync {
		t.Fatalf("expected no sync on cache hit")
	}
}

func TestHandleCacheMissUpdates(t *testing.T) {
	dir := t.TempDir()
	cache := signaturecache.New(dir, time.Hour, 10)
	data := []byte("hi")
	id := device.DeviceIdentity{
		SizeBytes:    uint64(len(data)),
		KernelUUID:   "kernel",
		GPTUUID:      "gpt",
		MBRSignature: "mbr",
		FSUUID:       "fs",
	}
	dev := &memDevice{buf: make([]byte, len(data)), id: id}
	srv, err := New(dev, zap.NewNop(), cache, "vg", "lv")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dgst, err := digest.SumReader(bytes.NewReader(data), digest.SHA256)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()
	stream := rsyncwire.NewStream(c1, maxFrame)

	var gbuf bytes.Buffer
	gbuf.WriteByte('G')
	gbuf.WriteByte(byte(len(digest.SHA256)))
	gbuf.WriteString(digest.SHA256)
	gbuf.Write(dgst[:])
	streamSend(t, ctx, stream, gbuf.Bytes())
	cl := rsyncwire.NewClient(stream)
	sendIdentity(t, ctx, cl, id)
	sendDelta(t, ctx, cl, 0, data)
	c1.Close()
	if err := waitHandle(t, errCh); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	cancel()
	if string(dev.buf) != string(data) {
		t.Fatalf("device %q want %q", dev.buf, data)
	}
	if !dev.sync {
		t.Fatalf("expected sync")
	}
	if _, ok := cache.Get("vg", "lv", int64(len(data))); !ok {
		t.Fatalf("expected cache populated")
	}
}

func TestHandleCacheTTLExpiry(t *testing.T) {
	dir := t.TempDir()
	cache := signaturecache.New(dir, 10*time.Millisecond, 10)
	data := []byte("hello")
	dgst, err := digest.SumReader(bytes.NewReader(data), digest.SHA256)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if err := cache.Put("vg", "lv", int64(len(data)), dgst[:]); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	dev := &memDevice{buf: append([]byte{}, data...)}
	srv, err := New(dev, zap.NewNop(), cache, "vg", "lv")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()
	var buf bytes.Buffer
	buf.WriteByte('G')
	buf.WriteByte(byte(len(digest.SHA256)))
	buf.WriteString(digest.SHA256)
	buf.Write(dgst[:])
	streamSend(t, ctx, rsyncwire.NewStream(c1, maxFrame), buf.Bytes())
	c1.Close()
	if err := waitHandle(t, errCh); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	cancel()
	if !dev.sync {
		t.Fatalf("expected sync after TTL expiry")
	}
}

func TestHandleCacheLRUEviction(t *testing.T) {
	dir := t.TempDir()
	cache := signaturecache.New(dir, time.Hour, 1)

	run := func(vg, lv, content string) {
		dev := &memDevice{buf: []byte(content)}
		dgst, err := digest.SumReader(bytes.NewReader([]byte(content)), digest.SHA256)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}
		srv, err := New(dev, zap.NewNop(), cache, vg, lv)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		c1, c2 := net.Pipe()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errCh := make(chan error, 1)
		go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()
		var buf bytes.Buffer
		buf.WriteByte('G')
		buf.WriteByte(byte(len(digest.SHA256)))
		buf.WriteString(digest.SHA256)
		buf.Write(dgst[:])
		streamSend(t, ctx, rsyncwire.NewStream(c1, maxFrame), buf.Bytes())
		c1.Close()
		if err := waitHandle(t, errCh); err != nil {
			t.Fatalf("Handle: %v", err)
		}
	}

	run("vg", "a", "a")
	if _, err := os.Stat(filepath.Join(dir, "vg", "a.sig")); err != nil {
		t.Fatalf("expected a.sig present: %v", err)
	}
	run("vg", "b", "b")
	if _, err := os.Stat(filepath.Join(dir, "vg", "a.sig")); !os.IsNotExist(err) {
		t.Fatalf("expected a.sig evicted, got %v", err)
	}
}

func TestHandleDeltaOffsetOverflow(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{buf: make([]byte, 1)}
	id := device.DeviceIdentity{
		SizeBytes:    uint64(len(dev.buf)),
		KernelUUID:   "kernel",
		GPTUUID:      "gpt",
		MBRSignature: "mbr",
		FSUUID:       "fs",
	}
	dev.id = id
	core, logs := observer.New(zap.ErrorLevel)
	srv, err := New(dev, zap.New(core), nil, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	stream := rsyncwire.NewStream(c1, maxFrame)
	cl := rsyncwire.NewClient(stream)
	sendIdentity(t, ctx, cl, id)
	var buf bytes.Buffer
	buf.WriteByte('D')
	binary.Write(&buf, binary.BigEndian, uint64(math.MaxInt64)+1)
	buf.WriteByte(0x1)
	streamSend(t, ctx, stream, buf.Bytes())
	c1.Close()
	if err := waitHandle(t, errCh); err == nil || !strings.Contains(err.Error(), "delta offset overflows int64") {
		t.Fatalf("expected overflow error, got %v", err)
	}
	cancel()
	entries := logs.FilterMessage("delta_out_of_bounds").All()
	if len(entries) != 1 {
		t.Fatalf("expected log, got %v", logs.All())
	}
	ctxMap := entries[0].ContextMap()
	if v, ok := ctxMap["offset_bytes"].(uint64); !ok || v != uint64(math.MaxInt64)+1 {
		t.Fatalf("unexpected offset_bytes %v", ctxMap["offset_bytes"])
	}
	if v, ok := ctxMap["delta_size_bytes"].(int64); !ok || v != 1 {
		t.Fatalf("unexpected delta_size_bytes %v", ctxMap["delta_size_bytes"])
	}
	if v, ok := ctxMap["device_size_bytes"].(int64); !ok || v != dev.Size() {
		t.Fatalf("unexpected device_size_bytes %v", ctxMap["device_size_bytes"])
	}
}

func TestHandleDeltaOutOfBounds(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{buf: make([]byte, 10)}
	id := device.DeviceIdentity{
		SizeBytes:    uint64(len(dev.buf)),
		KernelUUID:   "kernel",
		GPTUUID:      "gpt",
		MBRSignature: "mbr",
		FSUUID:       "fs",
	}
	dev.id = id
	core, logs := observer.New(zap.ErrorLevel)
	srv, err := New(dev, zap.New(core), nil, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	cl := rsyncwire.NewClient(rsyncwire.NewStream(c1, maxFrame))
	sendIdentity(t, ctx, cl, id)
	sendDelta(t, ctx, cl, 9, []byte("ab"))
	c1.Close()
	if err := waitHandle(t, errCh); err == nil || !strings.Contains(err.Error(), "delta out of bounds") {
		t.Fatalf("expected out of bounds error, got %v", err)
	}
	cancel()
	entries := logs.FilterMessage("delta_out_of_bounds").All()
	if len(entries) != 1 {
		t.Fatalf("expected log, got %v", logs.All())
	}
	ctxMap := entries[0].ContextMap()
	if v, ok := ctxMap["offset_bytes"].(int64); !ok || v != 9 {
		t.Fatalf("unexpected offset_bytes %v", ctxMap["offset_bytes"])
	}
	if v, ok := ctxMap["delta_size_bytes"].(int64); !ok || v != 2 {
		t.Fatalf("unexpected delta_size_bytes %v", ctxMap["delta_size_bytes"])
	}
	if v, ok := ctxMap["end_offset_bytes"].(int64); !ok || v != 11 {
		t.Fatalf("unexpected end_offset_bytes %v", ctxMap["end_offset_bytes"])
	}
	if v, ok := ctxMap["device_size_bytes"].(int64); !ok || v != 10 {
		t.Fatalf("unexpected device_size_bytes %v", ctxMap["device_size_bytes"])
	}
}

func TestHandleUnknownFrameType(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{buf: make([]byte, 1)}
	srv, err := New(dev, zap.NewNop(), nil, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	streamSend(t, ctx, rsyncwire.NewStream(c1, maxFrame), []byte{'X'})
	c1.Close()
	if err := waitHandle(t, errCh); err == nil || !strings.Contains(err.Error(), "unknown frame type") {
		t.Fatalf("expected unknown frame error, got %v", err)
	}
	cancel()
}

func TestHandleDigestMismatch(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{buf: make([]byte, 2)}
	id := device.DeviceIdentity{
		SizeBytes:    uint64(len(dev.buf)),
		KernelUUID:   "kernel",
		GPTUUID:      "gpt",
		MBRSignature: "mbr",
		FSUUID:       "fs",
	}
	dev.id = id
	core, logs := observer.New(zap.ErrorLevel)
	srv, err := New(dev, zap.New(core), nil, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	cl := rsyncwire.NewClient(rsyncwire.NewStream(c1, maxFrame))
	sendIdentity(t, ctx, cl, id)
	data := []byte("hi")
	sendDelta(t, ctx, cl, 0, data)
	bad, err := digest.SumReader(bytes.NewReader([]byte("ho")), digest.SHA256)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	sendDigest(t, ctx, cl, digest.SHA256, bad)
	c1.Close()
	if err := waitHandle(t, errCh); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
	cancel()
	entries := logs.FilterMessage("digest_mismatch").All()
	if len(entries) != 1 {
		t.Fatalf("expected digest mismatch log, got %v", logs.All())
	}
	ctxMap := entries[0].ContextMap()
	actual, err := digest.SumReader(bytes.NewReader(data), digest.SHA256)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if v, ok := ctxMap["algorithm"].(string); !ok || v != digest.SHA256 {
		t.Fatalf("unexpected algorithm %v", ctxMap["algorithm"])
	}
	if v, ok := ctxMap["expected_digest"].(string); !ok || v != fmt.Sprintf("%x", bad) {
		t.Fatalf("unexpected expected_digest %v", ctxMap["expected_digest"])
	}
	if v, ok := ctxMap["actual_digest"].(string); !ok || v != fmt.Sprintf("%x", actual) {
		t.Fatalf("unexpected actual_digest %v", ctxMap["actual_digest"])
	}
}

func TestHandleMissingIdentity(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{buf: make([]byte, 1)}
	srv, err := New(dev, zap.NewNop(), nil, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	cl := rsyncwire.NewClient(rsyncwire.NewStream(c1, maxFrame))
	sendDelta(t, ctx, cl, 0, []byte("a"))
	c1.Close()
	if err := waitHandle(t, errCh); err == nil || !strings.Contains(err.Error(), "precondition") {
		t.Fatalf("expected precondition error, got %v", err)
	}
	cancel()
}

func TestHandleIdentityMismatch(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	id := device.DeviceIdentity{
		SizeBytes:    1,
		KernelUUID:   "kernel",
		GPTUUID:      "gpt",
		MBRSignature: "mbr",
		FSUUID:       "fs",
	}
	dev := &memDevice{buf: make([]byte, 1), id: id}
	srv, err := New(dev, zap.NewNop(), nil, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	cl := rsyncwire.NewClient(rsyncwire.NewStream(c1, maxFrame))
	// Send mismatched identity (size 2 instead of 1).
	remote := id
	remote.SizeBytes = 2
	sendIdentity(t, ctx, cl, remote)
	c1.Close()
	if err := waitHandle(t, errCh); err == nil || !strings.Contains(err.Error(), "precondition") {
		t.Fatalf("expected precondition error, got %v", err)
	}
	cancel()
}

func TestHandleIdentityIgnoresMajorMinor(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	id := device.DeviceIdentity{
		SizeBytes:     1,
		KernelUUID:    "k",
		GPTUUID:       "g",
		MBRSignature:  "m",
		FSUUID:        "f",
		Major:         1,
		Minor:         1,
		ManifestEpoch: 2,
	}
	dev := &memDevice{buf: make([]byte, int(id.SizeBytes)), id: id}
	srv, err := New(dev, zap.NewNop(), nil, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	cl := rsyncwire.NewClient(rsyncwire.NewStream(c1, maxFrame))
	remote := id
	remote.Major++
	remote.Minor++
	sendIdentity(t, ctx, cl, remote)
	exp, err := digest.SumReader(bytes.NewReader(dev.buf), digest.SHA256)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	sendDigest(t, ctx, cl, digest.SHA256, exp)
	c1.Close()
	if err := waitHandle(t, errCh); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	cancel()
}

func TestHandleIdentityMismatchFields(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	id := device.DeviceIdentity{
		SizeBytes:     1,
		KernelUUID:    "k",
		GPTUUID:       "g",
		MBRSignature:  "m",
		FSUUID:        "f",
		Major:         1,
		Minor:         1,
		ManifestEpoch: 2,
	}
	dev := &memDevice{buf: make([]byte, int(id.SizeBytes)), id: id}
	srv, err := New(dev, zap.NewNop(), nil, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsyncwire.NewStream(c2, maxFrame)) }()

	cl := rsyncwire.NewClient(rsyncwire.NewStream(c1, maxFrame))
	remote := id
	remote.FSUUID = "other"
	sendIdentity(t, ctx, cl, remote)
	c1.Close()
	if err := waitHandle(t, errCh); err == nil || err.Error() != "precondition: device identity mismatch" {
		t.Fatalf("expected precondition: device identity mismatch, got %v", err)
	}
}
