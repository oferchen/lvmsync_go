package humanize

import (
	"errors"
	"strconv"
	"strings"
)

var units = map[string]uint64{
	"":   1,
	"B":  1,
	"K":  1024,
	"KB": 1024,
	"M":  1024 * 1024,
	"MB": 1024 * 1024,
	"G":  1024 * 1024 * 1024,
	"GB": 1024 * 1024 * 1024,
}

func ParseBytes(s string) (uint64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	for unit, mult := range units {
		if strings.HasSuffix(s, unit) {
			num := strings.TrimSuffix(s, unit)
			f, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0, err
			}
			return uint64(f * float64(mult)), nil
		}
	}
	return 0, errors.New("unknown size")
}
