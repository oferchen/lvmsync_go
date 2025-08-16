package config

import (
	"math"
	"testing"

	"github.com/pierrec/lz4/v4"
)

func TestLZ4Level(t *testing.T) {
	tests := []struct {
		name    string
		level   int
		want    uint32
		wantErr bool
	}{
		{name: "fast", level: int(lz4.Fast), want: uint32(lz4.Fast)},
		{name: "level3", level: int(lz4.Level3), want: uint32(lz4.Level3)},
		{name: "negative", level: -1, wantErr: true},
		{name: "overflow", level: int(math.MaxUint32) + 1, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LZ4Level(tc.level)
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
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}
