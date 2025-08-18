package device

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/moby/sys/mountinfo"
	"go.uber.org/zap"

	"lvmsync_go/internal/privilege"
)

func TestGetUUIDCanceledContext(t *testing.T) {
	info := NewInfo()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := info.GetUUID(ctx, "/dev/null"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

func TestGetUUIDStub(t *testing.T) {
	info := NewInfoWithDeps(func(context.Context, string) (string, error) {
		return "stub-uuid", nil
	}, nil, nil, nil, nil)
	got, err := info.GetUUID(context.Background(), "/dev/sda")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "stub-uuid" {
		t.Fatalf("expected stub-uuid, got %q", got)
	}
}

func TestGetUUIDError(t *testing.T) {
	wantErr := errors.New("fail")
	info := NewInfoWithDeps(func(ctx context.Context, path string) (string, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatalf("expected context with deadline")
		}
		if path != "/dev/fail" {
			t.Fatalf("unexpected path %q", path)
		}
		return "", wantErr
	}, nil, nil, nil, nil)
	if _, err := info.GetUUID(context.Background(), "/dev/fail"); !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestGetDeviceIDPrefersLVM(t *testing.T) {
	info := NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "blkid-id", nil },
		func(context.Context, string) (string, error) { return "lv-id", nil },
		nil,
		nil,
		nil,
	)
	got, err := info.GetDeviceID(context.Background(), "/dev/lvm0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "lv-id" {
		t.Fatalf("expected lv-id, got %q", got)
	}
}

func TestIDsMatch(t *testing.T) {
	info := NewInfoWithDeps(
		func(_ context.Context, path string) (string, error) {
			if strings.Contains(path, "src") {
				return "id1", nil
			}
			if strings.Contains(path, "dest") {
				return "id2", nil
			}
			return "same", nil
		}, nil, nil, nil, nil)
	match, err := info.IDsMatch(context.Background(), "/dev/src", "/dev/dest")
	if err != nil {
		t.Fatalf("IDsMatch: %v", err)
	}
	if match {
		t.Fatalf("expected mismatch")
	}
	match, err = info.IDsMatch(context.Background(), "/dev/a", "/dev/b")
	if err != nil {
		t.Fatalf("IDsMatch: %v", err)
	}
	if !match {
		t.Fatalf("expected match")
	}
}

func TestSetMountFunc(t *testing.T) {
	info := NewInfo()
	orig := reflect.ValueOf(info.mountFunc).Pointer()
	stub := func(context.Context, string) (bool, error) { return true, nil }
	prev := info.SetMountFunc(stub)
	if reflect.ValueOf(prev).Pointer() != orig {
		t.Fatalf("expected previous function to be original")
	}
	if reflect.ValueOf(info.mountFunc).Pointer() != reflect.ValueOf(stub).Pointer() {
		t.Fatalf("mountFunc not replaced")
	}
	info.SetMountFunc(prev)
}

func TestIsMountedRW(t *testing.T) {
	tests := []struct {
		name string
		val  bool
	}{
		{name: "mounted", val: true},
		{name: "unmounted", val: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			info := NewInfoWithDeps(nil, nil, func(context.Context, string) (bool, error) { return tt.val, nil }, nil, nil)
			got, err := info.IsMountedRW(context.Background(), "/dev/sda")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.val {
				t.Fatalf("expected %v, got %v", tt.val, got)
			}
		})
	}
}

func TestIsMountedRWError(t *testing.T) {
	want := errors.New("boom")
	info := NewInfoWithDeps(nil, nil, func(ctx context.Context, _ string) (bool, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatalf("expected context with deadline")
		}
		return false, want
	}, nil, nil)
	if _, err := info.IsMountedRW(context.Background(), "/dev/sda"); !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
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

			info := NewInfo()
			prev := info.SetMountFunc(mountFuncFromMountInfoFile(mounts.Name()))
			defer info.SetMountFunc(prev)

			got, err := info.IsMountedRW(context.Background(), dev.Name())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestFirstBlockDigest(t *testing.T) {
	want := [32]byte{1, 2, 3}
	info := NewInfoWithDeps(nil, nil, nil, func(ctx context.Context, path string, size uint64) ([32]byte, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatalf("expected context with deadline")
		}
		if path != "dev" || size != 123 {
			t.Fatalf("unexpected args %q %d", path, size)
		}
		return want, nil
	}, nil)
	got, err := info.FirstBlockDigest(context.Background(), "dev", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestFirstBlockDigestError(t *testing.T) {
	want := errors.New("boom")
	info := NewInfoWithDeps(nil, nil, nil, func(context.Context, string, uint64) ([32]byte, error) {
		return [32]byte{}, want
	}, nil)
	if _, err := info.FirstBlockDigest(context.Background(), "dev", 1); !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
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

	info := NewInfo()
	prev := info.SetMountFunc(mountFuncFromMountInfoFile(mounts.Name()))
	defer info.SetMountFunc(prev)

	got, err := info.IsMountedRW(context.Background(), dev.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("expected mounted read-write")
	}
}

func TestDefaultMountFuncMultipleRecords(t *testing.T) {
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
	line1 := fmt.Sprintf("42 24 0:0 / /mnt/ro ro,relatime - ext4 %s rw\n", escaped)
	line2 := fmt.Sprintf("43 24 0:0 / /mnt/rw rw,relatime - ext4 %s rw\n", escaped)
	if _, err := mounts.WriteString(line1 + line2); err != nil {
		t.Fatalf("write mountinfo: %v", err)
	}
	mounts.Close()
	defer os.Remove(mounts.Name())

	info := NewInfo()
	prev := info.SetMountFunc(mountFuncFromMountInfoFile(mounts.Name()))
	defer info.SetMountFunc(prev)

	got, err := info.IsMountedRW(context.Background(), dev.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("expected mounted read-write")
	}
}

func TestDefaultMountFuncBindMount(t *testing.T) {
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
	base := fmt.Sprintf("42 24 0:0 / /mnt/src ro,relatime - ext4 %s ro\n", escaped)
	bind := fmt.Sprintf("43 42 0:0 /mnt/src /mnt/bind rw,relatime - ext4 %s rw\n", escaped)
	if _, err := mounts.WriteString(base + bind); err != nil {
		t.Fatalf("write mountinfo: %v", err)
	}
	mounts.Close()
	defer os.Remove(mounts.Name())

	info := NewInfo()
	prev := info.SetMountFunc(mountFuncFromMountInfoFile(mounts.Name()))
	defer info.SetMountFunc(prev)

	got, err := info.IsMountedRW(context.Background(), dev.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("expected mounted read-write")
	}
}

func TestDefaultMountFuncBindMountMountpoint(t *testing.T) {
	dev, err := os.CreateTemp("", "dev")
	if err != nil {
		t.Fatalf("create device: %v", err)
	}
	defer os.Remove(dev.Name())
	dev.Close()

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	bind := filepath.Join(dir, "bind")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.Mkdir(bind, 0o755); err != nil {
		t.Fatalf("mkdir bind: %v", err)
	}

	mounts, err := os.CreateTemp("", "mountinfo")
	if err != nil {
		t.Fatalf("create mountinfo: %v", err)
	}
	escaped := strings.ReplaceAll(dev.Name(), " ", "\\040")
	base := fmt.Sprintf("42 24 0:0 / %s ro,relatime - ext4 %s ro\n", src, escaped)
	bm := fmt.Sprintf("43 42 0:0 %s %s rw,relatime - ext4 %s rw\n", src, bind, escaped)
	if _, err := mounts.WriteString(base + bm); err != nil {
		t.Fatalf("write mountinfo: %v", err)
	}
	mounts.Close()
	defer os.Remove(mounts.Name())

	info := NewInfo()
	prev := info.SetMountFunc(mountFuncFromMountInfoFile(mounts.Name()))
	defer info.SetMountFunc(prev)

	got, err := info.IsMountedRW(context.Background(), src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("expected mounted read-write")
	}
}

func TestDefaultMountFuncDuplicateEntries(t *testing.T) {
	dev, err := os.CreateTemp("", "dev")
	if err != nil {
		t.Fatalf("create device: %v", err)
	}
	defer os.Remove(dev.Name())
	dev.Close()

	dir := t.TempDir()
	mp := filepath.Join(dir, "mnt")
	if err := os.Mkdir(mp, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mounts, err := os.CreateTemp("", "mountinfo")
	if err != nil {
		t.Fatalf("create mountinfo: %v", err)
	}
	escaped := strings.ReplaceAll(dev.Name(), " ", "\\040")
	line1 := fmt.Sprintf("42 24 0:0 / %s ro,relatime - ext4 %s rw\n", mp, escaped)
	line2 := fmt.Sprintf("43 24 0:0 / %s rw,relatime - ext4 %s rw\n", mp, escaped)
	if _, err := mounts.WriteString(line1 + line2); err != nil {
		t.Fatalf("write mountinfo: %v", err)
	}
	mounts.Close()
	defer os.Remove(mounts.Name())

	info := NewInfo()
	prev := info.SetMountFunc(mountFuncFromMountInfoFile(mounts.Name()))
	defer info.SetMountFunc(prev)

	got, err := info.IsMountedRW(context.Background(), mp)
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
	info := NewInfo()
	prev := info.SetMountFunc(mountFuncFromMountInfoFile(nonexistent))
	defer info.SetMountFunc(prev)

	if _, err := info.IsMountedRW(context.Background(), dev.Name()); err == nil {
		t.Fatalf("expected error when reading mountinfo file")
	}
}

func mountFuncFromMountInfoFile(p string) func(context.Context, string) (bool, error) {
	return func(_ context.Context, path string) (bool, error) {
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
			if mi.Source != real && mi.Mountpoint != real && mi.Root != real {
				continue
			}
			for _, opt := range strings.Split(mi.Options, ",") {
				if opt == "rw" {
					return true, nil
				}
			}
		}
		return false, nil
	}
}

func TestDefaultLVMUUIDFunc(t *testing.T) {
	dir := t.TempDir()
	lvs := filepath.Join(dir, "lvs")
	const want = "0000-1111"
	script := fmt.Sprintf("#!/bin/sh\necho %s\n", want)
	if err := os.WriteFile(lvs, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+":"+oldPath)
	defer os.Setenv("PATH", oldPath)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := defaultLVMUUIDFunc(ctx, "/dev/test")
	if err != nil {
		t.Fatalf("defaultLVMUUIDFunc: %v", err)
	}
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSizeBytes(t *testing.T) {
	dev := &stubDevice{size: 123}
	info := NewInfo()
	prev := info.SetDetectFunc(func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *Runner) (Device, error) {
		return dev, nil
	})
	defer info.SetDetectFunc(prev)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := info.SizeBytes(ctx, "/dev/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 123 {
		t.Fatalf("SizeBytes() = %d, want 123", got)
	}
}

func TestSizeBytesCloseError(t *testing.T) {
	want := errors.New("close boom")
	dev := &stubDevice{size: 456, closeErr: want}
	info := NewInfo()
	prev := info.SetDetectFunc(func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *Runner) (Device, error) {
		return dev, nil
	})
	defer info.SetDetectFunc(prev)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := info.SizeBytes(ctx, "/dev/fake")
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
	if got != 456 {
		t.Fatalf("SizeBytes() = %d, want 456", got)
	}
}

type stubDevice struct {
	size     uint64
	closeErr error
}

func (s *stubDevice) Path() string                                     { return "" }
func (s *stubDevice) SizeBytes() uint64                                { return s.size }
func (s *stubDevice) BlockSize() uint64                                { return 0 }
func (s *stubDevice) Snapshot(context.Context, string) (Device, error) { return nil, nil }
func (s *stubDevice) Cleanup(context.Context) error                    { return nil }
func (s *stubDevice) Close() error                                     { return s.closeErr }
func (s *stubDevice) Identity() (DeviceIdentity, error) {
	return DeviceIdentity{SizeBytes: s.size}, nil
}
func (s *stubDevice) AppendWAL(r Range) error               { return nil }
func (s *stubDevice) RecoverWAL(fn func(Range) error) error { return nil }
