package device

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/sys/mountinfo"
)

func TestGetUUIDCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GetUUID(ctx, "/dev/null"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

func TestGetUUIDStub(t *testing.T) {
	prev := SetUUIDFunc(func(ctx context.Context, path string) (string, error) {
		return "stub-uuid", nil
	})
	defer SetUUIDFunc(prev)

	got, err := GetUUID(context.Background(), "/dev/sda")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "stub-uuid" {
		t.Fatalf("expected stub-uuid, got %q", got)
	}
}

func TestGetDeviceIDPrefersLVM(t *testing.T) {
	prevLVM := SetLVMUUIDFunc(func(context.Context, string) (string, error) { return "lv-id", nil })
	defer SetLVMUUIDFunc(prevLVM)
	called := false
	prev := SetUUIDFunc(func(context.Context, string) (string, error) { called = true; return "blkid-id", nil })
	defer SetUUIDFunc(prev)

	got, err := GetDeviceID(context.Background(), "/dev/lvm0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "lv-id" {
		t.Fatalf("expected lv-id, got %q", got)
	}
	if called {
		t.Fatalf("expected blkid lookup to be skipped")
	}
}

func TestIDsMatch(t *testing.T) {
	prevLVM := SetLVMUUIDFunc(func(context.Context, string) (string, error) { return "", errors.New("no lvm") })
	defer SetLVMUUIDFunc(prevLVM)
	prev := SetUUIDFunc(func(_ context.Context, path string) (string, error) {
		if strings.Contains(path, "src") {
			return "id1", nil
		}
		if strings.Contains(path, "dest") {
			return "id2", nil
		}
		return "", nil
	})
	defer SetUUIDFunc(prev)

	match, err := IDsMatch(context.Background(), "/dev/src", "/dev/dest")
	if err != nil {
		t.Fatalf("IDsMatch: %v", err)
	}
	if match {
		t.Fatalf("expected mismatch")
	}

	prev2 := SetUUIDFunc(func(_ context.Context, path string) (string, error) { return "same", nil })
	defer SetUUIDFunc(prev2)
	match, err = IDsMatch(context.Background(), "/dev/a", "/dev/b")
	if err != nil {
		t.Fatalf("IDsMatch: %v", err)
	}
	if !match {
		t.Fatalf("expected match")
	}
}

func TestIsMountedRW(t *testing.T) {
	tests := []struct {
		name string
		val  bool
	}{
		{"mounted", true},
		{"unmounted", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := SetMountFunc(func(path string) (bool, error) { return tt.val, nil })
			defer SetMountFunc(prev)

			got, err := IsMountedRW("/dev/sda")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.val {
				t.Fatalf("expected %v, got %v", tt.val, got)
			}
		})
	}
}

func TestDefaultMountFunc(t *testing.T) {
	cases := []struct {
		name string
		opts string
		want bool
	}{
		{name: "read-write", opts: "rw", want: true},
		{name: "read-only", opts: "ro", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dev, err := os.CreateTemp("", "dev")
			if err != nil {
				t.Fatalf("create device: %v", err)
			}
			defer os.Remove(dev.Name())
			dev.Close()

			mounts, err := os.CreateTemp("", "mountinfo")
			if err != nil {
				t.Fatalf("create mountinfo: %v", err)
			}
			escaped := strings.ReplaceAll(dev.Name(), " ", "\\040")
			line := fmt.Sprintf("42 24 0:0 / /mnt/test %s,relatime - ext4 %s rw\n", tc.opts, escaped)
			if _, err := mounts.WriteString(line); err != nil {
				t.Fatalf("write mountinfo: %v", err)
			}
			mounts.Close()
			defer os.Remove(mounts.Name())

			prev := SetMountFunc(mountFuncFromMountInfoFile(mounts.Name()))
			defer SetMountFunc(prev)

			got, err := IsMountedRW(dev.Name())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestDefaultMountFuncSpecialChars(t *testing.T) {
	dev, err := os.CreateTemp("", "dev with space")
	if err != nil {
		t.Fatalf("create device: %v", err)
	}
	defer os.Remove(dev.Name())
	dev.Close()

	mounts, err := os.CreateTemp("", "mountinfo")
	if err != nil {
		t.Fatalf("create mountinfo: %v", err)
	}
	escaped := strings.ReplaceAll(dev.Name(), " ", "\\040")
	line := fmt.Sprintf("42 24 0:0 / /mnt/test rw,foo=bar\\040baz - ext4 %s rw\n", escaped)
	if _, err := mounts.WriteString(line); err != nil {
		t.Fatalf("write mountinfo: %v", err)
	}
	mounts.Close()
	defer os.Remove(mounts.Name())

	prev := SetMountFunc(mountFuncFromMountInfoFile(mounts.Name()))
	defer SetMountFunc(prev)

	got, err := IsMountedRW(dev.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("expected mounted read-write")
	}
}

func TestDefaultMountFuncError(t *testing.T) {
	dev, err := os.CreateTemp("", "dev")
	if err != nil {
		t.Fatalf("create device: %v", err)
	}
	defer os.Remove(dev.Name())
	dev.Close()

	nonexistent := filepath.Join(os.TempDir(), "does-not-exist")
	prev := SetMountFunc(mountFuncFromMountInfoFile(nonexistent))
	defer SetMountFunc(prev)

	if _, err := IsMountedRW(dev.Name()); err == nil {
		t.Fatalf("expected error when reading mountinfo file")
	}
}

func mountFuncFromMountInfoFile(p string) func(string) (bool, error) {
	return func(path string) (bool, error) {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return false, err
		}
		f, err := os.Open(p)
		if err != nil {
			return false, err
		}
		defer f.Close()
		infos, err := mountinfo.GetMountsFromReader(f, nil)
		if err != nil {
			return false, err
		}
		for _, mi := range infos {
			if mi.Source == real {
				for _, opt := range strings.Split(mi.Options, ",") {
					if opt == "rw" {
						return true, nil
					}
				}
				return false, nil
			}
		}
		return false, nil
	}
}
