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

func TestNewDeduplicationStrategy(t *testing.T) {
	cfg := &config.Config{DedupStrategy: "bloom", DedupStateFile: "state"}
	if _, ok := NewDeduplicationStrategy(cfg).(*BloomFilterDedup); !ok {
		t.Fatal("expected bloom filter strategy")
	}

	cfg.DedupStrategy = "checksum"
	if _, ok := NewDeduplicationStrategy(cfg).(*ChecksumDedup); !ok {
		t.Fatal("expected checksum strategy")
	}
}
