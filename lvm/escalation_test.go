package lvm

import (
	"os/exec"
	"reflect"
	"testing"
)

func TestParseEscalation(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"sudo -n", []string{"sudo", "-n"}},
		{"\"/usr/bin/sudo wrapper\" -p 'my prompt' -n", []string{"/usr/bin/sudo wrapper", "-p", "my prompt", "-n"}},
	}
	for _, tc := range cases {
		got, err := ParseEscalation(tc.in)
		if err != nil {
			t.Fatalf("ParseEscalation(%q): %v", tc.in, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("ParseEscalation(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	bad := []string{"", "sudo -n | rm -rf /", "'unterminated"}
	for _, in := range bad {
		if _, err := ParseEscalation(in); err == nil {
			t.Fatalf("ParseEscalation(%q) expected error", in)
		}
	}
}

func TestVerifyEscalationCommandSplit(t *testing.T) {
	var name string
	var args []string
	origExec := execCommand
	execCommand = func(n string, a ...string) *exec.Cmd {
		name = n
		args = append([]string(nil), a...)
		return exec.Command("true")
	}
	defer func() { execCommand = origExec }()
	origUID := geteuid
	geteuid = func() int { return 1 }
	defer func() { geteuid = origUID }()
	esc := "\"/usr/bin/sudo wrapper\" -p 'my prompt' -n"
	if err := VerifyEscalationCommand(esc); err != nil {
		t.Fatalf("VerifyEscalationCommand: %v", err)
	}
	wantArgs := []string{"-p", "my prompt", "-n", "true"}
	if name != "/usr/bin/sudo wrapper" || !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("got %q %v, want %q %v", name, args, "/usr/bin/sudo wrapper", wantArgs)
	}
}
