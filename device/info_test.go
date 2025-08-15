package device

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

			mounts, err := os.CreateTemp("", "mounts")
			if err != nil {
				t.Fatalf("create mounts: %v", err)
			}
			if _, err := fmt.Fprintf(mounts, "%s /mnt/test ext4 %s,relatime 0 0\n", dev.Name(), tc.opts); err != nil {
				t.Fatalf("write mounts: %v", err)
			}
			mounts.Close()
			defer os.Remove(mounts.Name())

			prev := SetMountFunc(mountFuncFromFile(mounts.Name()))
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

func TestDefaultMountFuncError(t *testing.T) {
	dev, err := os.CreateTemp("", "dev")
	if err != nil {
		t.Fatalf("create device: %v", err)
	}
	defer os.Remove(dev.Name())
	dev.Close()

	nonexistent := filepath.Join(os.TempDir(), "does-not-exist")
	prev := SetMountFunc(mountFuncFromFile(nonexistent))
	defer SetMountFunc(prev)

	if _, err := IsMountedRW(dev.Name()); err == nil {
		t.Fatalf("expected error when reading mounts file")
	}
}

func mountFuncFromFile(mountsPath string) func(string) (bool, error) {
	return func(path string) (bool, error) {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return false, err
		}
		f, err := os.Open(mountsPath)
		if err != nil {
			return false, err
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 4 {
				continue
			}
			if fields[0] == real {
				for _, opt := range strings.Split(fields[3], ",") {
					if opt == "rw" {
						return true, nil
					}
				}
				return false, nil
			}
		}
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
}
