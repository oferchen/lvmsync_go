package common_test

import (
	"bufio"
	"bytes"
	"reflect"
	"testing"

	"lvmsync_go/common"
)

func TestHandshakeRoundTrip(t *testing.T) {
	original := common.Handshake{Version: common.ProtocolVersion, Compress: "gzip", Checksum: true}
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
	h := common.Handshake{Compress: "none", Checksum: true, ChecksumDedup: true}
	expected := "lvmsync PROTO[3] checksum-dedup compress:none"
	if h.String() != expected {
		t.Fatalf("unexpected string: %s", h.String())
	}
}
