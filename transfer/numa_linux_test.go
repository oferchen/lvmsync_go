//go:build linux

package transfer

import "testing"

func TestParseCPUList(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"0-2,4,6-7", []int{0, 1, 2, 4, 6, 7}},
		{"1", []int{1}},
		{"", nil},
	}
	for _, c := range cases {
		got := parseCPUList(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("len mismatch for %q: %v vs %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("mismatch for %q at %d: %d vs %d", c.in, i, got[i], c.want[i])
			}
		}
	}
}
