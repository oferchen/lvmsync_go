package sizeparse

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"unicode"
)

var maxUint64 = new(big.Int).SetUint64(math.MaxUint64)

// Parse parses a size string with optional units (KB, MB, GB, etc.) or a percentage
// (e.g., "10%") and returns the numeric value. If the value represents a
// percentage, the returned bool is true and the numeric value is the percentage
// as an integer. Otherwise the numeric value represents bytes.
func Parse(input string) (uint64, bool, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0, false, fmt.Errorf("empty size")
	}
	s = strings.ToUpper(s)

	if strings.HasSuffix(s, "%") {
		num := strings.TrimSpace(strings.TrimSuffix(s, "%"))
		r := new(big.Rat)
		if _, ok := r.SetString(num); !ok || !r.IsInt() {
			return 0, true, fmt.Errorf("invalid percentage value %q", input)
		}
		if r.Sign() < 0 {
			return 0, true, fmt.Errorf("negative percentage value %q", input)
		}
		i := r.Num()
		if i.Cmp(maxUint64) > 0 {
			return 0, true, fmt.Errorf("percentage value %q overflows uint64", input)
		}
		return i.Uint64(), true, nil
	}

	s = strings.ReplaceAll(s, " ", "")
	idx := 0
	for ; idx < len(s); idx++ {
		r := rune(s[idx])
		if !unicode.IsDigit(r) && r != '.' && !(idx == 0 && r == '-') {
			break
		}
	}
	numStr := s[:idx]
	unit := s[idx:]

	r := new(big.Rat)
	if _, ok := r.SetString(numStr); !ok {
		return 0, false, fmt.Errorf("invalid size %q", input)
	}
	if r.Sign() < 0 {
		return 0, false, fmt.Errorf("negative size %q", input)
	}

	mult := uint64(0)
	switch unit {
	case "", "B":
		mult = 1
	case "K", "KB":
		mult = 1e3
	case "M", "MB":
		mult = 1e6
	case "G", "GB":
		mult = 1e9
	case "T", "TB":
		mult = 1e12
	case "P", "PB":
		mult = 1e15
	case "E", "EB":
		mult = 1e18
	case "KIB":
		mult = 1 << 10
	case "MIB":
		mult = 1 << 20
	case "GIB":
		mult = 1 << 30
	case "TIB":
		mult = 1 << 40
	case "PIB":
		mult = 1 << 50
	case "EIB":
		mult = 1 << 60
	default:
		return 0, false, fmt.Errorf("unknown size suffix %q", unit)
	}

	r.Mul(r, new(big.Rat).SetUint64(mult))
	if !r.IsInt() {
		return 0, false, fmt.Errorf("size %q has fractional bytes", input)
	}
	i := r.Num()
	if i.Cmp(maxUint64) > 0 {
		return 0, false, fmt.Errorf("size %q overflows uint64", input)
	}
	return i.Uint64(), false, nil
}

// FormatBytes returns a human readable string for the given number of bytes.
func FormatBytes(b uint64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	n := float64(b)
	exp := 0
	for n >= unit && exp < len(suffixes) {
		n /= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", n, suffixes[exp-1])
}

var suffixes = []string{"kB", "MB", "GB", "TB", "PB", "EB"}
