package transfer

import (
	"bytes"
	"testing"

	"lvmsync_go/config"
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
	cfg := &config.Config{DedupStrategy: "bloom", DedupStateFile: "state"}
	if _, ok := NewDeduplicationStrategy(cfg).(*BloomFilterDedup); !ok {
		t.Fatal("expected bloom filter strategy")
	}

	cfg.DedupStrategy = "checksum"
	if _, ok := NewDeduplicationStrategy(cfg).(*ChecksumDedup); !ok {
		t.Fatal("expected checksum strategy")
	}

	origDetect := detectBestStrategy
	defer func() { detectBestStrategy = origDetect }()

	cfg.DedupStrategy = "auto"
	detectBestStrategy = func() string { return "rolling_hash" }
	if _, ok := NewDeduplicationStrategy(cfg).(*RollingHashDedup); !ok {
		t.Fatal("expected rolling hash strategy for auto")
	}

	cfg.DedupStrategy = "auto"
	detectBestStrategy = func() string { return "checksum" }
	if _, ok := NewDeduplicationStrategy(cfg).(*ChecksumDedup); !ok {
		t.Fatal("expected checksum strategy for auto")
	}
}
