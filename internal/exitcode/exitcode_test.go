package exitcode

import "testing"

func TestExitCodeValues(t *testing.T) {
	cases := []struct {
		name string
		code int
		want int
	}{
		{"OK", OK, 0},
		{"ErrCapability", ErrCapability, 10},
		{"ErrDevice", ErrDevice, 20},
		{"ErrSnapshotExhausted", ErrSnapshotExhausted, 25},
		{"ErrPlatform", ErrPlatform, 30},
		{"ErrConfig", ErrConfig, 40},
		{"ErrRuntime", ErrRuntime, 50},
		{"ErrVerify", ErrVerify, 60},
		{"ErrPartial", ErrPartial, 70},
		{"ErrPrecondition", ErrPrecondition, 80},
		{"ErrResumable", ErrResumable, 90},
	}
	for _, c := range cases {
		if c.code != c.want {
			t.Errorf("%s = %d; want %d", c.name, c.code, c.want)
		}
	}
}
