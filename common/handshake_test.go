package common_test

import (
	"bufio"
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/oferchen/lvmsync_go/common"
)

func TestHandshakeRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		original common.Handshake
	}{
		{
			name: "minimal with alpn and tls",
			original: common.Handshake{
				Version:    common.ProtocolVersion,
				Compress:   "none",
				ALPN:       "h2",  // expect ALPN "h2" to round-trip
				TLSVersion: "1.3", // expect TLS version "1.3" to round-trip
			},
		},
		{
			name: "full handshake",
			original: common.Handshake{
				Version:       common.ProtocolVersion,
				Compress:      "gzip",
				CompressLevel: 2,
				Checksum:      true,
				Endianness:    common.NativeEndianness(),
				BlockSize:     4096,
				DedupMode:     "fixed",
				ResumeToken:   "token",
				ODirect:       true,
				MaxInFlight:   8,
				CDCMin:        1024,
				CDCAvg:        2048,
				CDCMax:        4096,
				ALPN:          "h2",  // expect ALPN "h2" to round-trip
				TLSVersion:    "1.3", // expect TLS version "1.3" to round-trip
				Transports:    []string{"ssh", "tcp+tls"},
				Compressors:   []string{"lz4", "zstd"},
				Digests:       []string{"sha256", "blake3"},
				Transport:     "ssh",
				Digest:        "blake3",
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := common.WriteHandshake(&buf, tt.original); err != nil {
				t.Fatalf("write handshake: %v", err)
			}
			parsed, err := common.ReadHandshake(bufio.NewReader(&buf))
			if err != nil {
				t.Fatalf("read handshake: %v", err)
			}
			if !reflect.DeepEqual(tt.original, parsed) {
				t.Fatalf("handshake mismatch: %+v != %+v", tt.original, parsed)
			}
		})
	}
}

func TestHandshakeString(t *testing.T) {
	h := common.Handshake{Transports: []string{"ssh"}, Compress: "none", Digests: []string{"blake3"}, Digest: "blake3", Checksum: true, ChecksumDedup: true}
	expected := "lvmsync PROTO[3] transports:ssh digests:blake3 checksum-dedup compress:none digest:blake3"
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

func TestHandshakeIgnoresUnknownTokens(t *testing.T) {
	line := "lvmsync PROTO[3] transport:ssh foo bar baz:qux compress:none digest:sha256\n"
	h, err := common.ReadHandshake(bufio.NewReader(strings.NewReader(line)))
	if err != nil {
		t.Fatalf("read handshake: %v", err)
	}
	expected := common.Handshake{Version: common.ProtocolVersion, Transport: "ssh", Compress: "none", Digest: "sha256"}
	if !reflect.DeepEqual(h, expected) {
		t.Fatalf("handshake mismatch: %+v != %+v", h, expected)
	}
}

func TestHandshakeMalformedTokens(t *testing.T) {
	cases := []struct {
		token     string
		errSubstr string
	}{
		{"block:not-a-number", "invalid block size"},
		{"level:abc", "invalid compression level"},
		{"inflight:xyz", "invalid max in-flight"},
		{"cdcmin:foo", "invalid cdc min"},
		{"cdcavg:bar", "invalid cdc avg"},
		{"cdcmax:baz", "invalid cdc max"},
	}
	for _, tt := range cases {
		line := "lvmsync PROTO[3] " + tt.token + "\n"
		if _, err := common.ReadHandshake(bufio.NewReader(strings.NewReader(line))); err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
			t.Fatalf("expected error containing %q for token %q, got %v", tt.errSubstr, tt.token, err)
		}
	}
}
