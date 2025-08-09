package transfer

import (
	"bufio"
	"bytes"
	"testing"

	"lvmsync_go/common"
	"lvmsync_go/config"
)

func TestComposeHandshake(t *testing.T) {
	cfg := &config.Config{Compress: "zstd"}
	hs := composeHandshake(cfg, "")
	if hs.Compress != "zstd" || hs.Checksum || hs.ChecksumDedup {
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
	common.WriteHandshake(buf, common.Handshake{Checksum: true})
	hs, err := readAndValidateHandshake(bufio.NewReader(buf), nil, true)
	if err != nil || !hs.Checksum {
		t.Fatalf("expected valid handshake, got %v %v", hs, err)
	}
}

func TestReadAndValidateHandshakeError(t *testing.T) {
	buf := &bytes.Buffer{}
	common.WriteHandshake(buf, common.Handshake{Checksum: false})
	_, err := readAndValidateHandshake(bufio.NewReader(buf), nil, true)
	if err == nil {
		t.Fatal("expected error for missing checksum")
	}
}
