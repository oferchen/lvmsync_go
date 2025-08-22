package exitcode

import "testing"

func TestExitCodeValues(t *testing.T) {
	cases := []struct {
		name string
		code int
		want int
	}{
		{"OK", OK, 0},
		{"Capability", Capability, 10},
		{"Device", Device, 20},
		{"SnapshotExhausted", SnapshotExhausted, 25},
		{"Platform", Platform, 30},
		{"Config", Config, 40},
		{"Runtime", Runtime, 50},
		{"Verify", Verify, 60},
		{"Partial", Partial, 70},
		{"Precondition", Precondition, 80},
		{"Resumable", Resumable, 90},
	}
	for _, c := range cases {
		if c.code != c.want {
			t.Errorf("%s = %d; want %d", c.name, c.code, c.want)
		}
	}
}
