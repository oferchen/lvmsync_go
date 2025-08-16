package transfer

import (
	"bufio"
	"bytes"
	"testing"

	"lvmsync_go/common"
	"lvmsync_go/config"
)

func TestComposeHandshake(t *testing.T) {
	cfg := &config.Config{Compress: "zstd", Transport: "ssh", ChecksumAlgorithm: "sha256"}
	hs := composeHandshake(cfg, "")
	if hs.Compress != "zstd" || len(hs.Transports) != 1 || hs.Transports[0] != "ssh" {
		t.Fatalf("unexpected handshake: %+v", hs)
	}
	hs = composeHandshake(cfg, StrategyChecksum)
	if !hs.Checksum || hs.ChecksumDedup {
		t.Fatalf("checksum handshake not set correctly")
	}
	hs = composeHandshake(cfg, StrategyChecksum+"-dedup")
	if !hs.Checksum || !hs.ChecksumDedup {
		t.Fatalf("checksum-dedup handshake not set correctly")
	}
}

func TestReadAndValidateHandshake(t *testing.T) {
	buf := &bytes.Buffer{}
	common.WriteHandshake(buf, common.Handshake{Transports: []string{"ssh"}, Compress: "zstd", Compressors: []string{"zstd"}, Digests: []string{"sha256"}, Checksum: true, CDCMin: 64, CDCAvg: 128, CDCMax: 256, ResumeToken: "tok", MaxInFlight: 8})
	cfg := &config.Config{Transport: "ssh", Compress: "zstd", ChecksumAlgorithm: "sha256"}
	hs, err := readAndValidateHandshake(cfg, bufio.NewReader(buf), nil, true)
	if err != nil || !hs.Checksum {
		t.Fatalf("expected valid handshake, got %v %v", hs, err)
	}
	if cfg.CDCMin != 64 || cfg.CDCAvg != 128 || cfg.CDCMax != 256 || cfg.ResumeToken != "tok" || cfg.Concurrency != 8 {
		t.Fatalf("values not propagated: %+v", cfg)
	}
}

func TestReadAndValidateHandshakeError(t *testing.T) {
	buf := &bytes.Buffer{}
	common.WriteHandshake(buf, common.Handshake{Transports: []string{"ssh"}, Compress: "zstd", Compressors: []string{"zstd"}, Digests: []string{"sha256"}, Checksum: false})
	cfg := &config.Config{Transport: "ssh", Compress: "zstd", ChecksumAlgorithm: "sha256"}
	_, err := readAndValidateHandshake(cfg, bufio.NewReader(buf), nil, true)
	if err == nil {
		t.Fatal("expected error for missing checksum")
	}
}

func TestReadAndValidateResumeTokenMismatch(t *testing.T) {
	buf := &bytes.Buffer{}
	common.WriteHandshake(buf, common.Handshake{Transports: []string{"ssh"}, Compress: "zstd", Compressors: []string{"zstd"}, Digests: []string{"sha256"}, ResumeToken: "remote"})
	cfg := &config.Config{Transport: "ssh", Compress: "zstd", ChecksumAlgorithm: "sha256", ResumeToken: "local"}
	if _, err := readAndValidateHandshake(cfg, bufio.NewReader(buf), nil, false); err == nil {
		t.Fatal("expected resume token mismatch error")
	}
}

func TestReadAndValidateMaxInFlightMismatch(t *testing.T) {
	buf := &bytes.Buffer{}
	common.WriteHandshake(buf, common.Handshake{Transports: []string{"ssh"}, Compress: "zstd", Compressors: []string{"zstd"}, Digests: []string{"sha256"}, MaxInFlight: 4})
	cfg := &config.Config{Transport: "ssh", Compress: "zstd", ChecksumAlgorithm: "sha256", Concurrency: 8}
	if _, err := readAndValidateHandshake(cfg, bufio.NewReader(buf), nil, false); err == nil {
		t.Fatal("expected max in-flight mismatch error")
	}
}

func TestHandshakeNegotiation(t *testing.T) {
	buf := &bytes.Buffer{}
	// remote supports ssh and tcp+tls, prefers ssh
	common.WriteHandshake(buf, common.Handshake{Transports: []string{"ssh", "tcp+tls"}, Compressors: []string{"lz4", "zstd"}, Digests: []string{"sha256", "blake3"}, Checksum: true})
	cfg := &config.Config{Transport: "tcp+tls,ssh", Compress: "zstd,lz4", ChecksumAlgorithm: "blake3,sha256"}
	hs, err := readAndValidateHandshake(cfg, bufio.NewReader(buf), nil, true)
	if err != nil {
		t.Fatalf("negotiation failed: %v", err)
	}
	if cfg.Transport != "tcp+tls" || cfg.Compress != "zstd" || cfg.ChecksumAlgorithm != "blake3" {
		t.Fatalf("unexpected negotiation result: %+v", cfg)
	}
	if hs.Transports[0] != "tcp+tls" || hs.Compress != "zstd" || hs.Digests[0] != "blake3" {
		t.Fatalf("unexpected handshake result: %+v", hs)
	}
}

func TestHandshakeDigestMismatch(t *testing.T) {
	buf := &bytes.Buffer{}
	common.WriteHandshake(buf, common.Handshake{Transports: []string{"ssh"}, Compressors: []string{"zstd"}, Digests: []string{"sha256"}, Checksum: true})
	cfg := &config.Config{Transport: "ssh", Compress: "zstd", ChecksumAlgorithm: "blake3"}
	if _, err := readAndValidateHandshake(cfg, bufio.NewReader(buf), nil, true); err == nil {
		t.Fatal("expected digest mismatch error")
	}
}
