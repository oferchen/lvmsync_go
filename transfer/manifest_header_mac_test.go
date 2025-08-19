package transfer

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/zeebo/blake3"

	manifestpkg "lvmsync_go/manifest"
)

func TestManifestHeaderMACSize(t *testing.T) {
	var hdr manifestpkg.Header
	hdr.Version = manifestpkg.Version
	hdr.BlockSize = 4096
	hdr.SizeBytes = 8192
	hdr.ChunkCount = 2
	hdr.MinChunkSize = 1
	hdr.AvgChunkSize = 2
	hdr.MaxChunkSize = 3
	hdr.HybridFixedSize = 4
	hdr.Epoch = 1
	hdr.Major = 1
	hdr.Minor = 2
	copy(hdr.DeviceID[:], []byte("dev"))
	digest := blake3.Sum256([]byte("data"))
	hdr.FirstBlockDigest = digest

	mac := manifestHeaderMAC(&hdr)

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, hdr); err != nil {
		t.Fatalf("binary.Write: %v", err)
	}
	size := binary.Size(hdr)
	expected := blake3.Sum256(buf.Bytes()[:size-32])
	if mac != expected {
		t.Fatalf("manifestHeaderMAC mismatch: got %x, want %x", mac, expected)
	}
}
