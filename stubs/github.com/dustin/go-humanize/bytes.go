package humanize

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseBytes(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	mult := uint64(1)
	if strings.HasSuffix(s, "K") {
		mult = 1024
		s = strings.TrimSuffix(s, "K")
	} else if strings.HasSuffix(s, "M") {
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "M")
	} else if strings.HasSuffix(s, "G") {
		mult = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "G")
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return n * mult, nil
}
