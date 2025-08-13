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
		ChecksumDedup: true,
		Endianness:    common.NativeEndianness(),
		BlockSize:     4096,
		DedupMode:     "fixed",
		ResumeToken:   "token",
		ODirect:       true,
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
	h := common.Handshake{
		Compress:      "none",
		Checksum:      true,
		ChecksumDedup: true,
		Endianness:    "little",
		BlockSize:     1024,
		DedupMode:     "cdc",
		ResumeToken:   "r",
		ODirect:       true,
		CompressLevel: 1,
	}
	expected := "lvmsync PROTO[3] endian:little block:1024 dedup:cdc resume:r odirect checksum-dedup compress:none level:1"
	if h.String() != expected {
		t.Fatalf("unexpected string: %s", h.String())
	}
}
