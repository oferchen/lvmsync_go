package transfer

import (
	"bytes"
	"testing"

	"github.com/bits-and-blooms/bloom/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"lvmsync_go/internal/config"
)

func TestWrapRateLimitedWriterDisabled(t *testing.T) {
	buf := &bytes.Buffer{}
	if WrapRateLimitedWriter(buf, 0) != buf {
		t.Fatal("writer should be unchanged when limit is 0")
	}
}

func TestWrapRateLimitedWriterEnabled(t *testing.T) {
	buf := &bytes.Buffer{}
	w := WrapRateLimitedWriter(buf, 1024)
	if w == buf {
		t.Fatal("writer should be wrapped when limit is positive")
	}
	data := []byte("test data")
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if got := buf.String(); got != string(data) {
		t.Fatalf("unexpected buffer content %q", got)
	}
}

func TestNewDeduplicationStrategy(t *testing.T) {
	cfg := &config.Config{DedupStrategy: "bloom", DedupStateFile: "state", BloomEntries: 1000, BloomFpRate: 0.05}
	d := NewDeduplicationStrategy(cfg, zap.NewNop())
	bf, ok := d.(*BloomFilterDedup)
	if !ok {
		t.Fatal("expected bloom filter strategy")
	}
	m, k := bloom.EstimateParameters(uint(cfg.BloomEntries), cfg.BloomFpRate)
	if bf.filter.Cap() != m || bf.filter.K() != k {
		t.Fatalf("unexpected bloom filter parameters m=%d k=%d want m=%d k=%d", bf.filter.Cap(), bf.filter.K(), m, k)
	}

	cfg.DedupStrategy = "checksum"
	if _, ok := NewDeduplicationStrategy(cfg, zap.NewNop()).(*ChecksumDedup); !ok {
		t.Fatal("expected checksum strategy")
	}

	cfg.DedupStrategy = "auto"
	if _, ok := NewDeduplicationStrategyWithDeps(cfg, zap.NewNop(), &Deps{DetectBestStrategy: func() string { return "rolling_hash" }}).(*RollingHashDedup); !ok {
		t.Fatal("expected rolling hash strategy for auto")
	}

	cfg.DedupStrategy = "auto"
	if _, ok := NewDeduplicationStrategyWithDeps(cfg, zap.NewNop(), &Deps{DetectBestStrategy: func() string { return "checksum" }}).(*ChecksumDedup); !ok {
		t.Fatal("expected checksum strategy for auto")
	}
}

func TestNewDeduplicationStrategyInvalidBloomEntries(t *testing.T) {
	cfg := &config.Config{DedupStrategy: "bloom", BloomEntries: -1, BloomFpRate: 0.05}
	if NewDeduplicationStrategy(cfg, zap.NewNop()) != nil {
		t.Fatal("expected nil strategy for invalid bloom entries")
	}
}

func TestNewTransfer(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), nil, nil)
	if tr.Logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if tr.Logger.Core().Enabled(zapcore.InfoLevel) {
		t.Fatal("expected nop logger")
	}
}
