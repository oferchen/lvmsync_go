package common

import (
	"bufio"
	"bytes"
	"reflect"
	"testing"
)

func TestHandshakeRoundTrip(t *testing.T) {
	original := Handshake{Version: ProtocolVersion, Compress: "gzip", Checksum: true}
	var buf bytes.Buffer
	if err := WriteHandshake(&buf, original); err != nil {
		t.Fatalf("write handshake: %v", err)
	}
	parsed, err := ReadHandshake(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("read handshake: %v", err)
	}
	if !reflect.DeepEqual(original, parsed) {
		t.Fatalf("handshake mismatch: %+v != %+v", original, parsed)
	}
}

func TestHandshakeString(t *testing.T) {
	h := Handshake{Compress: "none", Checksum: true, ChecksumDedup: true}
	expected := "lvmsync PROTO[3] checksum-dedup compress:none"
	if h.String() != expected {
		t.Fatalf("unexpected string: %s", h.String())
	}
}
