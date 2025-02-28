// transfer/metadata.go
package transfer

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ReadMetadataHeader(metadataPath string) (int64, error) {
	file, err := os.Open(metadataPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	buf := make([]byte, 16)
	if _, err := io.ReadFull(file, buf); err != nil {
		return 0, err
	}
	magic := binary.LittleEndian.Uint32(buf[0:4])
	valid := binary.LittleEndian.Uint32(buf[4:8])
	version := binary.LittleEndian.Uint32(buf[8:12])
	chunk := binary.LittleEndian.Uint32(buf[12:16])
	if magic != 0x70416e53 {
		return 0, fmt.Errorf("invalid snapshot magic number")
	}
	if valid != 1 {
		return 0, fmt.Errorf("snapshot is marked as invalid")
	}
	if version != 1 {
		return 0, fmt.Errorf("incompatible snapshot metadata version")
	}
	return int64(chunk) * 512, nil
}

func GetDifferences(metadataPath string, chunkSize int64) ([]Range, error) {
	file, err := os.Open(metadataPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if _, err := file.Seek(chunkSize, io.SeekStart); err != nil {
		return nil, err
	}
	var diffs []uint64
	buf := make([]byte, 16)
	for {
		_, err := io.ReadFull(file, buf)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return nil, err
		}
		originOffset := binary.LittleEndian.Uint64(buf[0:8])
		snapOffset := binary.LittleEndian.Uint64(buf[8:16])
		if snapOffset == 0 {
			break
		}
		diffs = append(diffs, originOffset)
	}
	var ranges []Range
	for _, block := range diffs {
		start := int64(block) * chunkSize
		end := (int64(block)+1)*chunkSize - 1
		ranges = append(ranges, Range{Start: start, End: end})
	}
	return ranges, nil
}

func GetMetadataDevice(snapshot string) string {
	base := filepath.Base(snapshot)
	parts := strings.SplitN(base, "-", 2)
	if len(parts) < 2 {
		return ""
	}
	vg := strings.ReplaceAll(parts[0], "-", "--")
	lv := strings.ReplaceAll(parts[1], "-", "--")
	return "/dev/mapper/" + vg + "-" + lv + "-cow"
}
