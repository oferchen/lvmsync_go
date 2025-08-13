package transfer

import (
	"bufio"
	"bytes"
	"testing"

	"lvmsync_go/common"
	"lvmsync_go/config"
)

func TestComposeHandshake(t *testing.T) {
	cfg := &config.Config{Compress: "zstd", CompressLevel: 1, BlockSize: 4096, DedupMode: "fixed", ResumeState: "r", ODirect: true}
	hs := composeHandshake(cfg, "")
	if hs.Compress != "zstd" || hs.CompressLevel != 1 || hs.BlockSize != 4096 || hs.DedupMode != "fixed" || hs.ResumeToken != "r" || !hs.ODirect || hs.Checksum || hs.ChecksumDedup {
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
	cfg := &config.Config{BlockSize: 4096, DedupMode: "fixed", Compress: "none", CompressLevel: 0, ODirect: true}
	h := common.Handshake{Checksum: true, BlockSize: 4096, DedupMode: "fixed", Compress: "none", ODirect: true, Endianness: common.NativeEndianness()}
	common.WriteHandshake(buf, h)
	hs, err := readAndValidateHandshake(bufio.NewReader(buf), cfg, nil, true)
	if err != nil || !hs.Checksum {
		t.Fatalf("expected valid handshake, got %v %v", hs, err)
	}
}

func TestReadAndValidateHandshakeError(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := &config.Config{Compress: "none"}
	common.WriteHandshake(buf, common.Handshake{Checksum: false, Compress: "none"})
	_, err := readAndValidateHandshake(bufio.NewReader(buf), cfg, nil, true)
	if err == nil {
		t.Fatal("expected error for missing checksum")
	}
}
