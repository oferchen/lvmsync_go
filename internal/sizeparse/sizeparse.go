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
	s := strings.TrimSpace(strings.ToUpper(input))
	if s == "" {
		return 0, false, fmt.Errorf("empty size")
	}

	if strings.HasSuffix(s, "%") {
		num := strings.TrimSpace(strings.TrimSuffix(s, "%"))
		r := new(big.Rat)
		if _, ok := r.SetString(num); !ok || !r.IsInt() {
			return 0, true, fmt.Errorf("invalid percentage value %q", input)
		}

		if f < 0 {
			return 0, true, fmt.Errorf("negative percentage value %q", input)
		}
		return f, true, nil

		i := r.Num()
		if i.Sign() < 0 || i.Cmp(maxUint64) > 0 {
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
	if f < 0 {
		return 0, false, fmt.Errorf("negative size %q", input)
	}


	mult := new(big.Rat)
	switch unit {
	case "", "B":
		mult.SetInt64(1)
	case "K", "KB":
		mult.SetInt64(1e3)
	case "M", "MB":
		mult.SetInt64(1e6)
	case "G", "GB":
		mult.SetInt64(1e9)
	case "T", "TB":
		mult.SetInt64(1e12)
	case "P", "PB":
		mult.SetInt64(1e15)
	case "E", "EB":
		mult.SetInt64(1e18)
		return f, false, nil
	case "K", "KB", "KIB":
		return f * (1 << 10), false, nil
	case "M", "MB", "MIB":
		return f * (1 << 20), false, nil
	case "G", "GB", "GIB":
		return f * (1 << 30), false, nil
	case "T", "TB", "TIB":
		return f * (1 << 40), false, nil
	case "P", "PB", "PIB":
		return f * (1 << 50), false, nil
	case "E", "EB", "EIB":
		return f * (1 << 60), false, nil
	default:
		return 0, false, fmt.Errorf("unknown size suffix %q", unit)
	}

	r.Mul(r, mult)
	if !r.IsInt() {
		return 0, false, fmt.Errorf("size %q has fractional bytes", input)
	}
	i := r.Num()
	if i.Sign() < 0 || i.Cmp(maxUint64) > 0 {
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
	div := float64(unit)
	exp := 0
	units := []string{"kB", "MB", "GB", "TB", "PB", "EB"}
	f := float64(b) / div
	for f >= unit && exp < len(units)-1 {
		f /= unit
		exp++
	}
	if f < 10 {
		return fmt.Sprintf("%.1f %s", f, units[exp])
	}
	return fmt.Sprintf("%.0f %s", f, units[exp])
}
