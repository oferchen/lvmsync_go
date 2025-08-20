package transfer

import (
	"context"
	"crypto/rand"
	"hash/maphash"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/zeebo/blake3"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/device"
	manifestpkg "lvmsync_go/manifest"
)

func TestBloomFilterDedupPersistence(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state")

	d1 := &BloomFilterDedup{filter: bloom.NewWithEstimates(1000, 0.01), stateFile: stateFile, entries: 1000, fpRate: 0.01, strategy: &SHA256Checksum{}, deps: DefaultDeps}
	data := []byte("hello")

	if !d1.ShouldTransfer(0, data) {
		t.Fatalf("first transfer should be allowed")
	}
	d1.RecordTransfer(0, data)
	if err := d1.SaveState(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	d2 := &BloomFilterDedup{filter: bloom.NewWithEstimates(1000, 0.01), stateFile: stateFile, entries: 1000, fpRate: 0.01, strategy: &SHA256Checksum{}, deps: DefaultDeps}
	if err := d2.loadState(); err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	if d2.ShouldTransfer(0, data) {
		t.Fatalf("expected bloom filter to persist state")
	}
}

func TestBloomFilterDedupStatsLog(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state")
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	d := &BloomFilterDedup{
		filter:    bloom.NewWithEstimates(10, 0.01),
		stateFile: stateFile,
		entries:   10,
		fpRate:    0.01,
		strategy:  &SHA256Checksum{},
		logger:    logger,
		deps:      DefaultDeps,
	}
	blocks := [][]byte{[]byte("a"), []byte("b")}
	for i, b := range blocks {
		if !d.ShouldTransfer(int64(i), b) {
			t.Fatalf("first transfer should be allowed")
		}
		d.RecordTransfer(int64(i), b)
	}
	if err := d.SaveState(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}
	logs := observed.FilterMessage("dedup_bloom_stats").All()
	if len(logs) != 1 {
		t.Fatalf("expected stats log, got %d", len(logs))
	}
	fields := logs[0].ContextMap()
	if entries, ok := fields["entries"].(uint64); !ok || entries != 2 {
		t.Fatalf("entries: got %v", fields["entries"])
	}
	if fp, ok := fields["configured_fp_rate"].(float64); !ok || fp != 0.01 {
		t.Fatalf("configured_fp_rate: got %v", fields["configured_fp_rate"])
	}
	if obs, ok := fields["observed_fp_rate"].(float64); !ok || obs != 0 {
		t.Fatalf("observed_fp_rate: got %v", fields["observed_fp_rate"])
	}
}

func TestRollingHashDedupPersistence(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state")

	d1 := &RollingHashDedup{stateFile: stateFile, hashes: make(map[int64]uint64), seed: maphash.MakeSeed(), deps: DefaultDeps}
	data := []byte("hello")

	if !d1.ShouldTransfer(0, data) {
		t.Fatalf("first transfer should be allowed")
	}
	d1.RecordTransfer(0, data)
	if err := d1.SaveState(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	d2 := &RollingHashDedup{stateFile: stateFile, hashes: make(map[int64]uint64), seed: maphash.MakeSeed(), deps: DefaultDeps}
	if err := d2.loadState(); err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	if d2.ShouldTransfer(0, data) {
		t.Fatalf("expected rolling hash dedup to persist state")
	}
}

func TestManifestIndexLifecycle(t *testing.T) {
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "dev-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	data := make([]byte, 8192)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	file.Close()

	manPath := filepath.Join(dir, "dev.man")
	info := device.NewInfoWithDeps(func(context.Context, string) (string, error) { return "id", nil }, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := manifestpkg.Rebuild(ctx, file.Name(), manPath, zap.NewNop(), 0, false, 0, 0, 0, 0, manifestpkg.WithDeviceInfo(info)); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	idx, err := manifestpkg.Open(manPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer idx.Close()
	off, length, _, _, digest, err := idx.Entry(0)
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	buf := make([]byte, length)
	f, err := os.Open(file.Name())
	if err != nil {
		t.Fatalf("open device: %v", err)
	}
	defer f.Close()
	if _, err := f.ReadAt(buf, int64(off)); err != nil {
		t.Fatalf("read: %v", err)
	}
	if blake3.Sum256(buf) != digest {
		t.Fatalf("digest mismatch")
	}
}
