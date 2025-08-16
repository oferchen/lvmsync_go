package config

import (
	"math"
	"testing"

	"lvmsync_go/internal/sizeparse"
)

func TestFormatBlockSize(t *testing.T) {
	overflow := uint64(math.MaxInt)
	overflow++

	tests := []struct {
		name    string
		input   int
		want    string
		wantErr bool
	}{
		{name: "negative", input: -1, wantErr: true},
		{name: "overflow", input: int(overflow), wantErr: true},
		{name: "valid", input: 1024, want: sizeparse.FormatBytes(1024)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FormatBlockSize(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
