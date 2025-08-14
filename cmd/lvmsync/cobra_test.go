package lvmsync

import (
	"os"
	"testing"

	verifycmd "lvmsync_go/cmd/verify"
)

func TestRunCommandExecutes(t *testing.T) {
	t.Setenv("LVMSYNC_TRANSPORT_TRANSPORT", "ssh")
	var gotSrc, gotDst string
	var gotOpts RunOptions
	runCommand = func(src, dst string, opts RunOptions) error {
		gotSrc, gotDst, gotOpts = src, dst, opts
		return nil
	}
	t.Cleanup(func() { runCommand = func(src, dst string, opts RunOptions) error { return nil } })

	if err := Execute([]string{"run", "src", "dst"}); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	if gotSrc != "src" || gotDst != "dst" {
		t.Fatalf("unexpected args %q %q", gotSrc, gotDst)
	}
	if gotOpts.DryRun {
		t.Fatalf("expected dry-run false")
	}
	if gotOpts.Transport != "ssh" {
		t.Fatalf("unexpected transport %q", gotOpts.Transport)
	}
}

func TestRunCommandDryRun(t *testing.T) {
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	called := false
	runCommand = func(src, dst string, opts RunOptions) error {
		called = true
		return nil
	}
	t.Cleanup(func() { runCommand = func(src, dst string, opts RunOptions) error { return nil } })

	if err := Execute([]string{"run", "--dry-run", src, "dst"}); err != nil {
		t.Fatalf("execute run dry-run: %v", err)
	}
	if called {
		t.Fatalf("runCommand should not be called in dry-run")
	}
}

func TestRunCommandDryRunEnv(t *testing.T) {
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	t.Setenv("LVMSYNC_DRY_RUN", "true")
	called := false
	runCommand = func(src, dst string, o RunOptions) error {
		called = true
		return nil
	}
	t.Cleanup(func() { runCommand = func(src, dst string, opts RunOptions) error { return nil } })

	if err := Execute([]string{"run", src, "dst"}); err != nil {
		t.Fatalf("execute run with env: %v", err)
	}
	if called {
		t.Fatalf("runCommand should not be called when dry-run env set")
	}
}

func TestManifestRebuildRoutes(t *testing.T) {
	var gotDevice string
	var dry bool
	manifestRebuild = func(device string, d bool) error {
		gotDevice, dry = device, d
		return nil
	}
	t.Cleanup(func() { manifestRebuild = func(device string, dryRun bool) error { return nil } })

	if err := Execute([]string{"manifest", "rebuild", "--dry-run", "/dev/vg0"}); err != nil {
		t.Fatalf("execute rebuild: %v", err)
	}
	if gotDevice != "/dev/vg0" {
		t.Fatalf("unexpected device %q", gotDevice)
	}
	if !dry {
		t.Fatalf("expected dry-run true")
	}
}

func TestVerifyRoutes(t *testing.T) {
	var got []string
	verifyRun = func(a []string) error {
		got = append([]string{}, a...)
		return nil
	}
	t.Cleanup(func() { verifyRun = func(args []string) error { return verifycmd.Run(args, nil) } })

	if err := Execute([]string{"verify", "a", "b"}); err != nil {
		t.Fatalf("execute verify: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected args %v", got)
	}
}
