package transport

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/common"
)

func TestHandshakeFields(t *testing.T) {
	hs := common.Handshake{
		DedupMode:     "cdc",
		BlockSize:     4096,
		Compress:      "zstd",
		CompressLevel: 1,
		Digest:        "sha256",
		ResumeToken:   "tok",
		Checksum:      true,
		ChecksumDedup: true,
		Endianness:    "little",
		ODirect:       true,
		MaxInFlight:   8,
		CDCMin:        64,
		CDCAvg:        128,
		CDCMax:        256,
		Transport:     "quic",
		ALPN:          "lvmsync",
		TLSVersion:    "1.3",
	}

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	logger.Info("handshake", HandshakeFields(hs)...)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	ctx := entries[0].ContextMap()
	expected := map[string]interface{}{
		"dedup_mode":       "cdc",
		"block_size_bytes": int64(4096),
		"compress":         "zstd",
		"compress_level":   int64(1),
		"digest":           "sha256",
		"resume_token":     "tok",
		"checksum":         true,
		"checksum_dedup":   true,
		"endianness":       "little",
		"odirect":          true,
		"max_inflight":     int64(8),
		"cdc_min":          int64(64),
		"cdc_avg":          int64(128),
		"cdc_max":          int64(256),
		"transport":        "quic",
		"alpn":             "lvmsync",
		"tls_version":      "1.3",
	}
	if len(ctx) != len(expected) {
		t.Fatalf("expected %d fields, got %d", len(expected), len(ctx))
	}
	for k, v := range expected {
		if val, ok := ctx[k]; !ok || val != v {
			t.Fatalf("missing or unexpected value for %s: got %v want %v", k, val, v)
		}
	}
}
