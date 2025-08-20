package device

import (
	"os"
	"testing"
)

func TestReadPartitionSignaturesMBR(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "mbr")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	buf := make([]byte, 512)
	// Disk signature 0x12345678 in little endian
	buf[440] = 0x78
	buf[441] = 0x56
	buf[442] = 0x34
	buf[443] = 0x12
	// MBR marker
	buf[510] = 0x55
	buf[511] = 0xaa
	if _, err := f.Write(buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	gpt, mbr, err := readPartitionSignatures(f.Name())
	if err != nil {
		t.Fatalf("readPartitionSignatures: %v", err)
	}
	if gpt != "" {
		t.Fatalf("expected empty gpt signature, got %q", gpt)
	}
	if mbr != "12345678" {
		t.Fatalf("expected mbr signature 12345678, got %q", mbr)
	}
}

func TestReadPartitionSignaturesGPT(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "gpt")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	buf := make([]byte, 1024)
	// Protective MBR signature
	buf[510] = 0x55
	buf[511] = 0xaa
	// MBR disk signature 0x12345678
	buf[440] = 0x78
	buf[441] = 0x56
	buf[442] = 0x34
	buf[443] = 0x12
	// GPT header at LBA1
	copy(buf[512:], []byte("EFI PART"))
	// Disk GUID 00112233-4455-6677-8899-aabbccddeeff
	hdr := buf[512:1024]
	guid := []byte{0x33, 0x22, 0x11, 0x00, 0x55, 0x44, 0x77, 0x66, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	copy(hdr[56:], guid)
	if _, err := f.Write(buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	gpt, mbr, err := readPartitionSignatures(f.Name())
	if err != nil {
		t.Fatalf("readPartitionSignatures: %v", err)
	}
	if gpt != "00112233-4455-6677-8899-aabbccddeeff" {
		t.Fatalf("unexpected gpt guid %q", gpt)
	}
	if mbr != "12345678" {
		t.Fatalf("unexpected mbr signature %q", mbr)
	}
}
