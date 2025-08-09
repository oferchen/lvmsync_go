package blocksize

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Detect determines an optimal block size for the given block device.
// It inspects sysfs queue attributes in the following order of preference:
//  1. optimal_io_size
//  2. minimum_io_size
//  3. physical_block_size
//  4. logical_block_size
//
// If none of these values are available or greater than zero, a default
// of 4096 bytes is returned.
func Detect(devicePath string) (int, error) {
	fi, err := os.Stat(devicePath)
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", devicePath, err)
	}
	stat, ok := fi.Sys().(*unix.Stat_t)
	if !ok {
		return 0, fmt.Errorf("unsupported file info for %s", devicePath)
	}
	queuePath := filepath.Join("/sys/dev/block", fmt.Sprintf("%d:%d/queue", unix.Major(stat.Rdev), unix.Minor(stat.Rdev)))
	return readPreferredSize(queuePath)
}

var preference = []string{"optimal_io_size", "minimum_io_size", "physical_block_size", "logical_block_size"}

func readPreferredSize(queuePath string) (int, error) {
	for _, name := range preference {
		data, err := os.ReadFile(filepath.Join(queuePath, name))
		if err != nil {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || v <= 0 {
			continue
		}
		return v, nil
	}
	return 4096, nil
}
