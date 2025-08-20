package device

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
)

// readPartitionSignatures returns the GPT disk GUID and MBR signature for the
// device at path. Empty strings are returned when a signature is not present.
func readPartitionSignatures(path string) (string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	var sector [512]byte
	if _, err := f.ReadAt(sector[:], 0); err != nil {
		return "", "", err
	}
	var mbr string
	if sector[510] == 0x55 && sector[511] == 0xaa {
		mbr = fmt.Sprintf("%08x", binary.LittleEndian.Uint32(sector[440:444]))
	}
	var gpt string
	if _, err := f.ReadAt(sector[:], 512); err == nil {
		if string(sector[:8]) == "EFI PART" {
			gpt = formatGUID(sector[56:72])
		}
	}
	return gpt, mbr, nil
}

func formatGUID(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	d1 := binary.LittleEndian.Uint32(b[0:4])
	d2 := binary.LittleEndian.Uint16(b[4:6])
	d3 := binary.LittleEndian.Uint16(b[6:8])
	d4 := b[8:10]
	d5 := hex.EncodeToString(b[10:16])
	return fmt.Sprintf("%08x-%04x-%04x-%02x%02x-%s", d1, d2, d3, d4[0], d4[1], d5)
}
