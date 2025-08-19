package sizeparse

import (
	"math"
	"strconv"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in          string
		want        uint64
		wantPercent bool
		wantErr     bool
	}{
		{"1KB", 1000, false, false},
		{"1MB", 1e6, false, false},
		{"1GB", 1e9, false, false},
		{"1KiB", 1 << 10, false, false},
		{"1MiB", 1 << 20, false, false},
		{"1GiB", 1 << 30, false, false},
		{"1.5GB", 3 * (1000 * 1000 * 1000) / 2, false, false},
		{"1.5GiB", 3 * (1 << 29), false, false},
		{"1.5KB", 1500, false, false},
		{"1.5MiB", 1572864, false, false},
		{"50%", 50, true, false},
		{"1%", 1, true, false},
		{"1.5%", 0, true, true},
		{"-10%", 0, true, true},
		{strconv.FormatUint(math.MaxUint64, 10) + "0%", 0, true, true},
		{"bad", 0, false, true},
		{"10XB", 0, false, true},
		{"-1G", 0, false, true},
		{"1.1B", 0, false, true},
		{"1.1MiB", 0, false, true},
		{strconv.FormatUint(math.MaxUint64, 10), math.MaxUint64, false, false},
		{strconv.FormatUint(math.MaxUint64, 10) + "B", math.MaxUint64, false, false},
		{"18.446744073709551615E", math.MaxUint64, false, false},
		{"18446744073709551616", 0, false, true},
		{"18446744073709551616B", 0, false, true},
		{"18446744073709551615K", 0, false, true},
		{"", 0, false, true},
	}

	for _, tc := range cases {
		got, pct, err := Parse(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("Parse(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", tc.in, err)
		}
		if pct != tc.wantPercent {
			t.Fatalf("Parse(%q) percent = %v, want %v", tc.in, pct, tc.wantPercent)
		}
		if got != tc.want {
			t.Fatalf("Parse(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	if got := FormatBytes(4000); got != "4.0 kB" {
		t.Fatalf("FormatBytes(4000) = %q, want %q", got, "4.0 kB")
	}
}
