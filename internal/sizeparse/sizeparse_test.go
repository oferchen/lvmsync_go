package sizeparse

import (
	"math"
	"strconv"
	"testing"
)

func TestParseUnits(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"1KB", 1000},
		{"1MB", 1e6},
		{"1GB", 1e9},
		{"1KiB", 1 << 10},
		{"1MiB", 1 << 20},
		{"1GiB", 1 << 30},
	}
	for _, tc := range cases {
		got, pct, err := Parse(tc.in)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", tc.in, err)
		}
		if pct {
			t.Fatalf("Parse(%q) reported percent", tc.in)
		}
		if got != tc.want {
			t.Fatalf("Parse(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseFractional(t *testing.T) {
	valid := []struct {
		in   string
		want uint64
	}{
		{"1.5KB", 1500},
		{"1.5MiB", 1572864},
	}
	for _, tc := range valid {
		got, pct, err := Parse(tc.in)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", tc.in, err)
		}
		if pct {
			t.Fatalf("Parse(%q) reported percent", tc.in)
		}
		if got != tc.want {
			t.Fatalf("Parse(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}

	invalid := []string{"1.1B", "1.1MiB"}
	for _, in := range invalid {
		if _, _, err := Parse(in); err == nil {
			t.Fatalf("Parse(%q) expected error", in)
		}
	}
}

func TestParsePercent(t *testing.T) {
	cases := []struct {
		in      string
		want    uint64
		wantErr bool
	}{
		{"50%", 50, false},
		{"1%", 1, false},
		{"1.5%", 0, true},
		{"-10%", 0, true},
	}
	for _, tc := range cases {
		got, pct, err := Parse(tc.in)
		if (err != nil) != tc.wantErr {
			t.Fatalf("Parse(%q) error=%v wantErr=%v", tc.in, err, tc.wantErr)
		}
		if tc.wantErr {
			continue
		}
		if !pct {
			t.Fatalf("Parse(%q) percent flag = false", tc.in)
		}
		if got != tc.want {
			t.Fatalf("Parse(%q) = %d want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseOverflowAndNegative(t *testing.T) {
	overflow := []string{
		"18446744073709551616", // MaxUint64 + 1
		"18446744073709551616B",
		"18446744073709551615K",
	}
	for _, in := range overflow {
		if _, _, err := Parse(in); err == nil {
			t.Fatalf("Parse(%q) expected overflow error", in)
		}
	}

	negative := []string{"-1", "-1B", "-1KB"}
	for _, in := range negative {
		if _, _, err := Parse(in); err == nil {
			t.Fatalf("Parse(%q) expected negative error", in)
		}
	}
}

func TestParseMaxUint64(t *testing.T) {
	maxStr := strconv.FormatUint(math.MaxUint64, 10)
	for _, in := range []string{maxStr, maxStr + "B"} {
		got, pct, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q) error=%v", in, err)
		}
		if pct {
			t.Fatalf("Parse(%q) percent flag = true", in)
		}
		if got != math.MaxUint64 {
			t.Fatalf("Parse(%q) = %d want %d", in, got, uint64(math.MaxUint64))
		}
	}
}

func TestFormatBytes(t *testing.T) {
	if got := FormatBytes(4000); got != "4.0 kB" {
		t.Fatalf("FormatBytes(4000) = %q, want %q", got, "4.0 kB")
	}
}
