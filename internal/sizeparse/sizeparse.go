package sizeparse

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Parse parses a size string with optional units (KB, MB, GB, etc.) or percentage (e.g., "10%")
// and returns the numeric value. If the value represents a percentage, the returned bool is true
// and the numeric value is the percentage. Otherwise the numeric value represents bytes.
func Parse(input string) (float64, bool, error) {
	s := strings.TrimSpace(strings.ToUpper(input))
	if s == "" {
		return 0, false, fmt.Errorf("empty size")
	}
	if strings.HasSuffix(s, "%") {
		num := strings.TrimSpace(strings.TrimSuffix(s, "%"))
		f, err := strconv.ParseFloat(num, 64)
		if err != nil {
			return 0, true, fmt.Errorf("invalid percentage value %q", input)
		}
		return f, true, nil
	}
	s = strings.ReplaceAll(s, " ", "")
	idx := 0
	for ; idx < len(s); idx++ {
		r := rune(s[idx])
		if !unicode.IsDigit(r) && r != '.' {
			break
		}
	}
	numStr := s[:idx]
	unit := s[idx:]
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid size %q", input)
	}
	switch unit {
	case "", "B":
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
