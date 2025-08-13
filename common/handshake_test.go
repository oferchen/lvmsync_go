package common_test

import (
	"bufio"
	"bytes"
	"reflect"
	"testing"

	"lvmsync_go/common"
)

func TestHandshakeRoundTrip(t *testing.T) {
	original := common.Handshake{
		Version:       common.ProtocolVersion,
		Compress:      "gzip",
		CompressLevel: 2,
		Checksum:      true,
		Endianness:    common.NativeEndianness(),
		BlockSize:     4096,
		DedupMode:     "fixed",
		ResumeToken:   "token",
		ODirect:       true,
		Transports:    []string{"ssh", "tcp+tls"},
		Compressors:   []string{"lz4", "zstd"},
		Digests:       []string{"sha256", "blake3"},
		Transport:     "ssh",
		Digest:        "blake3",
	}

	var buf bytes.Buffer
	if err := common.WriteHandshake(&buf, original); err != nil {
		t.Fatalf("write handshake: %v", err)
	}
	parsed, err := common.ReadHandshake(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("read handshake: %v", err)
	}
	if !reflect.DeepEqual(original, parsed) {
		t.Fatalf("handshake mismatch: %+v != %+v", original, parsed)
	}
}

func TestHandshakeString(t *testing.T) {
	h := common.Handshake{Transports: []string{"ssh"}, Compress: []string{"none"}, Digests: []string{"blake3"}, Checksum: true, ChecksumDedup: true}
	expected := "lvmsync PROTO[3] checksum-dedup transport:ssh compress:none digest:blake3"
	if h.String() != expected {
		t.Fatalf("unexpected string: %s", h.String())
	}
}

func TestSelectBest(t *testing.T) {
	local := []string{"zstd", "lz4"}
	remote := []string{"lz4", "gzip"}
	if best := common.SelectBest(local, remote); best != "lz4" {
		t.Fatalf("expected lz4, got %s", best)
	}
	if best := common.SelectBest(local, []string{"br"}); best != "zstd" {
		t.Fatalf("expected fallback zstd, got %s", best)
	}
	if best := common.SelectBest([]string{}, remote); best != "" {
		t.Fatalf("expected empty, got %s", best)
	}
}
