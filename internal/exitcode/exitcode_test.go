package exitcode

import "testing"

func TestExitCodeValues(t *testing.T) {
	cases := []struct {
		name string
		code int
		want int
	}{
		{"OK", OK, 0},
		{"Precondition", Precondition, 2},
		{"Verify", Verify, 3},
		{"Resumable", Resumable, 4},
		{"Config", Config, 5},
		{"Capability", Capability, 6},
	}
	for _, c := range cases {
		if c.code != c.want {
			t.Errorf("%s = %d; want %d", c.name, c.code, c.want)
		}
	}
}
