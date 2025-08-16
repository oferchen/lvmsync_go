package common

import "testing"

func TestMergeHandshakeSelectsBest(t *testing.T) {
	local := Handshake{Compressors: []string{"zstd", "lz4"}, Digests: []string{"sha256", "blake3"}}
	peer := Handshake{Compressors: []string{"lz4"}, Digests: []string{"blake3"}}
	merged := MergeHandshake(local, peer)
	if merged.Compress != "lz4" || merged.Digest != "blake3" {
		t.Fatalf("unexpected merge result: %+v", merged)
	}
}

func TestMergeHandshakePreservesLocal(t *testing.T) {
	local := Handshake{Compress: "zstd", Digest: "sha256", Compressors: []string{"zstd"}, Digests: []string{"sha256"}}
	peer := Handshake{Compressors: []string{"lz4"}, Digests: []string{"blake3"}}
	merged := MergeHandshake(local, peer)
	if merged.Compress != "zstd" || merged.Digest != "sha256" {
		t.Fatalf("unexpected merge result: %+v", merged)
	}
}
