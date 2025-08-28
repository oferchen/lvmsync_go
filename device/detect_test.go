package device

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/lvm"
)

func setupLoop(t *testing.T, size int64) (string, func()) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "loopback")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatalf("truncate: %v", err)
	}
	f.Close()

	out, err := exec.Command("losetup", "--show", "-f", f.Name()).Output()
	if err != nil {
		t.Skipf("losetup: %v", err)
	}
	loop := strings.TrimSpace(string(out))
	loop = filepath.Clean(loop)
	cleanup := func() { exec.Command("losetup", "-d", loop).Run() }
	return loop, cleanup
}

func TestDetectFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	dev, err := Detect(ctx, f.Name(), true, true, "", "", "", "", 0, 0, fakeEsc{}, logger, NewRunner())
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*FileDevice); !ok {
		t.Fatalf("expected FileDevice, got %T", dev)
	}
	dev.Close()

	entries := logs.FilterMessage("detect_device_success").All()
	found := false
	for _, e := range entries {
		if e.ContextMap()["device_type"] == "file" && e.ContextMap()["path"] == f.Name() {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected log entry for file detection")
	}
}

func TestDetectFileSymlink(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	link := filepath.Join(t.TempDir(), "filelink")
	if err := os.Symlink(f.Name(), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	dev, err := Detect(ctx, link, true, true, "", "", "", "", 0, 0, fakeEsc{}, zap.NewNop(), NewRunner())
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*FileDevice); !ok {
		t.Fatalf("expected FileDevice, got %T", dev)
	}
	dev.Close()
}

func TestDetectPathValidation(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	rel, err := filepath.Rel(wd, f.Name())
	if err != nil {
		t.Fatalf("rel: %v", err)
	}

	cases := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"absolute", f.Name(), false},
		{"relative", rel, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := WithForce(context.Background(), true)
			ctx = WithAllowOverwrite(ctx, true)
			ctx = WithYesIKnow(ctx, true)
			_, err := Detect(ctx, tc.path, true, true, "", "", "", "", 0, 0, fakeEsc{}, zap.NewNop(), NewRunner())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for path %s", tc.path)
				}
				if !strings.Contains(err.Error(), "precondition") {
					t.Fatalf("expected precondition error, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("detect: %v", err)
				}
			}
		})
	}
}

func TestDetectRaw(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	dev, err := Detect(ctx, loop, true, true, "", "", "", "", 0, 0, fakeEsc{}, logger, NewRunner())
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*RawDevice); !ok {
		t.Fatalf("expected RawDevice, got %T", dev)
	}
	dev.Close()

	// Expect LVM failure debug and Raw success info.
	lvmFail := false
	rawSuccess := false
	for _, e := range logs.All() {
		switch e.Message {
		case "detect_device_failed":
			if e.ContextMap()["device_type"] == "lvm" {
				lvmFail = true
			}
		case "detect_device_success":
			if e.ContextMap()["device_type"] == "raw" {
				rawSuccess = true
			}
		}
	}
	if !lvmFail || !rawSuccess {
		t.Fatalf("expected lvm fail and raw success logs, got %v", logs.All())
	}
}

func TestDetectRawSymlink(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	link := filepath.Join(t.TempDir(), "rawlink")
	if err := os.Symlink(loop, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	dev, err := Detect(ctx, link, true, true, "", "", "", "", 0, 0, fakeEsc{}, zap.NewNop(), NewRunner())
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*RawDevice); !ok {
		t.Fatalf("expected RawDevice, got %T", dev)
	}
	dev.Close()
}

func TestDetectLVMSymlink(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	vgDir := filepath.Join("/dev", "testvg")
	lvPath := filepath.Join(vgDir, "testlv")
	os.MkdirAll(vgDir, 0o755)
	os.Symlink(loop, lvPath)
	defer os.Remove(lvPath)
	defer os.Remove(vgDir)

	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	dev, err := Detect(ctx, lvPath, true, true, "", "", "", "", 0, 0, fakeEsc{}, zap.NewNop(), NewRunner())
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*RawDevice); !ok {
		t.Fatalf("expected RawDevice, got %T", dev)
	}
	dev.Close()
}

func TestDetectLVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	cmd := cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name == "blkid" {
			return exec.CommandContext(ctx, "sh", "-c", "echo LVM2_member")
		}
		return exec.CommandContext(ctx, name, args...)
	})
	runner := NewDeviceRunner(cmd)
	runner.openLVMOverride = func(ctx context.Context, p string, _ *lvm.FDCache, _ bool, _ bool, _ string, _ *zap.Logger) (*LVMDevice, error) {
		return &LVMDevice{path: p, logger: zap.NewNop(), runner: runner}, nil
	}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	dev, err := Detect(ctx, loop, true, true, "", "", "", "", 0, 0, fakeEsc{}, zap.NewNop(), runner)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*LVMDevice); !ok {
		t.Fatalf("expected LVMDevice, got %T", dev)
	}
	dev.Close()
}

func TestDetectRawCommandQuoting(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	var calls []struct {
		name string
		args []string
	}
	cmd := cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		calls = append(calls, struct {
			name string
			args []string
		}{name: name, args: append([]string(nil), args...)})
		return exec.CommandContext(ctx, "/bin/true")
	})
	freeze := "/bin/echo 'freeze path with spaces'"
	thaw := "/bin/echo 'thaw path with spaces'"
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	dev, err := Detect(ctx, loop, false, false, "raw", freeze, thaw, "", 0, 0, fakeEsc{}, zap.NewNop(), NewDeviceRunner(cmd))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if err := dev.Cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 exec calls, got %d", len(calls))
	}
	if calls[0].name != "/bin/echo" || len(calls[0].args) != 1 || calls[0].args[0] != "freeze path with spaces" {
		t.Fatalf("unexpected freeze command: %s %v", calls[0].name, calls[0].args)
	}
	if calls[1].name != "/bin/echo" || len(calls[1].args) != 1 || calls[1].args[0] != "thaw path with spaces" {
		t.Fatalf("unexpected thaw command: %s %v", calls[1].name, calls[1].args)
	}
	dev.Close()
}

func TestDetectRawPartitionMismatch(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	f, err := os.OpenFile(loop, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open loop: %v", err)
	}
	var buf [512]byte
	buf[440] = 0x78
	buf[441] = 0x56
	buf[442] = 0x34
	buf[443] = 0x12
	buf[510] = 0x55
	buf[511] = 0xaa
	if _, err := f.Write(buf[:]); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()
	ctx := WithPartitionSignatures(context.Background(), "deadbeef", "")
	ctx = WithForce(ctx, true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	if _, err := detectRawDevice(ctx, loop, true, true, "", "", 0, 0, fakeEsc{}, zap.NewNop(), NewRunner()); err == nil || !errors.Is(err, ErrPartitionMismatch) {
		t.Fatalf("expected partition mismatch error, got %v", err)
	}
}

func TestDetectRawPartitionMatch(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	f, err := os.OpenFile(loop, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open loop: %v", err)
	}
	var buf [512]byte
	buf[440] = 0x78
	buf[441] = 0x56
	buf[442] = 0x34
	buf[443] = 0x12
	buf[510] = 0x55
	buf[511] = 0xaa
	if _, err := f.Write(buf[:]); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()
	ctx := WithPartitionSignatures(context.Background(), "", "12345678")
	ctx = WithForce(ctx, true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	dev, err := detectRawDevice(ctx, loop, true, true, "", "", 0, 0, fakeEsc{}, zap.NewNop(), NewRunner())
	if err != nil {
		t.Fatalf("detectRawDevice: %v", err)
	}
	dev.Close()
}

func TestDetectPartitionComparison(t *testing.T) {
	cases := []struct {
		name    string
		sig1    uint32
		sig2    *uint32
		wantErr bool
	}{
		{name: "match", sig1: 0x12345678, sig2: ptr(uint32(0x12345678)), wantErr: false},
		{name: "mismatch", sig1: 0x12345678, sig2: ptr(uint32(0x87654321)), wantErr: true},
		{name: "missing", sig1: 0x12345678, sig2: nil, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f1 := createMBRFile(t, &tc.sig1)
			var f2 string
			if tc.sig2 != nil {
				f2 = createMBRFile(t, tc.sig2)
			} else {
				f2 = createMBRFile(t, nil)
			}
			ctx := WithPartitionSignatures(context.Background(), "", "")
			ctx = WithForce(ctx, true)
			ctx = WithAllowOverwrite(ctx, true)
			ctx = WithYesIKnow(ctx, true)
			dev, err := Detect(ctx, f1, true, true, "", "", "", "", 0, 0, fakeEsc{}, zap.NewNop(), NewRunner())
			if err != nil {
				t.Fatalf("detect src: %v", err)
			}
			dev.Close()
			_, err = Detect(ctx, f2, true, true, "", "", "", "", 0, 0, fakeEsc{}, zap.NewNop(), NewRunner())
			if tc.wantErr {
				if err == nil || !errors.Is(err, ErrPartitionMismatch) {
					t.Fatalf("expected partition mismatch error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("detect dst: %v", err)
			}
		})
	}
}

func createMBRFile(t *testing.T, sig *uint32) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mbr")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	var buf [512]byte
	if sig != nil {
		binary.LittleEndian.PutUint32(buf[440:444], *sig)
		buf[510] = 0x55
		buf[511] = 0xaa
	}
	if _, err := f.Write(buf[:]); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()
	return f.Name()
}

func ptr[T any](v T) *T { return &v }
