package transfer

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

func TestReadMetadataHeader(t *testing.T) {
	valid := make([]byte, 16)
	binary.LittleEndian.PutUint32(valid[0:4], 0x70416e53)
	binary.LittleEndian.PutUint32(valid[4:8], 1)
	binary.LittleEndian.PutUint32(valid[8:12], 1)
	binary.LittleEndian.PutUint32(valid[12:16], 8)
	tests := []struct {
		name     string
		header   []byte
		expected int64
		errSub   string
	}{
		{
			name:     "valid",
			header:   valid,
			expected: 8 * 512,
		},
		{
			name: "badMagic",
			header: func() []byte {
				h := make([]byte, 16)
				copy(h, valid)
				binary.LittleEndian.PutUint32(h[0:4], 0)
				return h
			}(),
			errSub: "invalid snapshot magic number",
		},
		{
			name: "invalidFlag",
			header: func() []byte {
				h := make([]byte, 16)
				copy(h, valid)
				binary.LittleEndian.PutUint32(h[4:8], 0)
				return h
			}(),
			errSub: "snapshot is marked as invalid",
		},
		{
			name: "badVersion",
			header: func() []byte {
				h := make([]byte, 16)
				copy(h, valid)
				binary.LittleEndian.PutUint32(h[8:12], 2)
				return h
			}(),
			errSub: "incompatible snapshot metadata version",
		},
		{
			name:   "shortHeader",
			header: valid[:10],
			errSub: "EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp(t.TempDir(), "meta")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			if _, err := tmpFile.Write(tt.header); err != nil {
				t.Fatalf("failed to write header: %v", err)
			}
			tmpFile.Close()

			size, err := ReadMetadataHeader(tmpFile.Name())
			if tt.errSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSub) {
					t.Fatalf("expected error containing %q, got %v", tt.errSub, err)
				}
			} else {
				if err != nil {
					t.Fatalf("ReadMetadataHeader returned error: %v", err)
				}
				if size != tt.expected {
					t.Fatalf("unexpected chunk size %d, want %d", size, tt.expected)
				}
			}
		})
	}
}

func TestGetMetadataDevice(t *testing.T) {
	tests := []struct {
		name     string
		snapshot string
		expected string
	}{
		{"simple", "vg-lv", "/dev/mapper/vg-lv-cow"},
		{"withPath", "/dev/mapper/vg-lv", "/dev/mapper/vg-lv-cow"},
		{"lvWithHyphen", "vg-lv-extra", "/dev/mapper/vg-lv--extra-cow"},
		{"missingDash", "snapshot", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetMetadataDevice(tt.snapshot); got != tt.expected {
				t.Fatalf("GetMetadataDevice(%q) = %q, want %q", tt.snapshot, got, tt.expected)
			}
		})
	}
}
