// transfer/metadata.go
package transfer

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

var mapperDir = "/dev/mapper"

// SetMapperDir overrides the directory used to look up mapper devices.
func SetMapperDir(dir string) {
	mapperDir = dir
}

func ReadMetadataHeader(metadataPath string) (chunkSize int64, err error) {
	file, err := os.Open(metadataPath)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			if err == nil {
				err = fmt.Errorf("close metadata file: %w", closeErr)
			} else {
				err = fmt.Errorf("%v; close metadata file: %w", err, closeErr)
			}
		}
	}()

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

func GetDifferences(metadataPath string, chunkSize int64) (ranges []Range, err error) {
	file, err := os.Open(metadataPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			if err == nil {
				err = fmt.Errorf("close metadata file: %w", closeErr)
			} else {
				err = fmt.Errorf("%v; close metadata file: %w", err, closeErr)
			}
		}
	}()

	if _, err = file.Seek(chunkSize, io.SeekStart); err != nil {
		return nil, err
	}

	buf := make([]byte, 16)

	for {
		_, err = io.ReadFull(file, buf)
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

		chunkSizeU := uint64(chunkSize)
		maxInt := uint64(math.MaxInt64)

		if originOffset > maxInt/chunkSizeU {
			return nil, fmt.Errorf("range start overflows int64")
		}
		if originOffset >= (maxInt+1)/chunkSizeU {
			return nil, fmt.Errorf("range end overflows int64")
		}

		start := originOffset * chunkSizeU
		end := (originOffset+1)*chunkSizeU - 1

		ranges = append(ranges, Range{Start: int64(start), End: int64(end)})
	}

	return ranges, nil
}

func GetMetadataDevice(snapshot string) string {
	base := filepath.Base(snapshot)
	parts := strings.SplitN(base, "-", 2)
	if len(parts) < 2 {
		return ""
	}

	replacer := strings.NewReplacer("-", "--")
	vg := replacer.Replace(parts[0])
	lv := replacer.Replace(parts[1])

	return filepath.Join(mapperDir, fmt.Sprintf("%s-%s-cow", vg, lv))
}
