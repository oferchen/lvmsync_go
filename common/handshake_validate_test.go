package common

import "testing"

func TestValidateHandshake(t *testing.T) {
	local := Handshake{DedupMode: "fixed", CDCMin: 64, CDCAvg: 128, CDCMax: 256, Compress: "zstd", CompressLevel: 1, ODirect: true, ResumeToken: "tok", MaxInFlight: 8, Endianness: "little"}
	peer := local
	if err := ValidateHandshake(local, peer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	peer.DedupMode = "cdc"
	if err := ValidateHandshake(local, peer); err == nil {
		t.Fatal("expected dedup mismatch error")
	}
}

func TestValidateHandshakeALPNMismatch(t *testing.T) {
	local := Handshake{ALPN: "lvmsync"}
	peer := local
	peer.ALPN = "other"
	if err := ValidateHandshake(local, peer); err == nil {
		t.Fatal("expected alpn mismatch error")
	}
}

func TestValidateHandshakeTLSVersionMismatch(t *testing.T) {
	local := Handshake{TLSVersion: "1.3"}
	peer := local
	peer.TLSVersion = "1.2"
	if err := ValidateHandshake(local, peer); err == nil {
		t.Fatal("expected tls version mismatch error")
	}
}
