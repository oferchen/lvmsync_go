package sizeparse

import (
	"math"
	"strconv"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input   string
		want    uint64
		percent bool
		wantErr bool
	}{
		{"1KB", float64(1 << 10), false, false},
		{"1MB", float64(1 << 20), false, false},
		{"1.5GB", 1.5 * float64(1<<30), false, false},
		{"1KiB", float64(1 << 10), false, false},
		{"1MiB", float64(1 << 20), false, false},
		{"50%", 50, true, false},
		{"bad", 0, false, true},
		{"10XB", 0, false, true},
		{strconv.FormatUint(math.MaxUint64, 10), math.MaxUint64, false, false},
		{strconv.FormatUint(math.MaxUint64, 10) + "B", math.MaxUint64, false, false},
		{"18.446744073709551615E", math.MaxUint64, false, false},
		{"18446744073709551616", 0, false, true},
		{"18446744073709551616B", 0, false, true},
		{"18446744073709551615K", 0, false, true},
	}
	for _, tt := range tests {
		got, pct, err := Parse(tt.input)
		if (err != nil) != tt.wantErr {
			t.Fatalf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if err == nil {
			if pct != tt.percent {
				t.Fatalf("Parse(%q) percent = %v, want %v", tt.input, pct, tt.percent)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	}
}

func TestFormatBytes(t *testing.T) {
	if got := FormatBytes(4000); got != "4.0 kB" {
		t.Fatalf("FormatBytes(4000) = %q, want %q", got, "4.0 kB")
	}
}
