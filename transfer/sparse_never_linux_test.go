//go:build linux

package transfer

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/internal/config"
)

func TestSparseNeverLoopback(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	if _, err := exec.LookPath("losetup"); err != nil {
		t.Skip("losetup not installed")
	}
	fsTypes := []struct {
		name string
		mkfs []string
	}{
		{"ext4", []string{"mkfs.ext4", "-F", "-q"}},
		{"xfs", []string{"mkfs.xfs", "-f"}},
	}
	for _, fs := range fsTypes {
		t.Run(fs.name, func(t *testing.T) {
			if _, err := exec.LookPath(fs.mkfs[0]); err != nil {
				t.Skipf("%s not installed", fs.mkfs[0])
			}
			dir := t.TempDir()
			img := filepath.Join(dir, "img")
			f, err := os.Create(img)
			if err != nil {
				t.Fatalf("create img: %v", err)
			}
			if err := f.Truncate(32 << 20); err != nil {
				t.Fatalf("truncate: %v", err)
			}
			f.Close()
			out, err := exec.Command("losetup", "--find", "--show", img).CombinedOutput()
			if err != nil {
				t.Skipf("losetup failed: %v %s", err, out)
			}
			loop := strings.TrimSpace(string(out))
			t.Cleanup(func() { exec.Command("losetup", "-d", loop).Run() })
			mkfsCmd := append(fs.mkfs, loop)
			if out, err := exec.Command(mkfsCmd[0], mkfsCmd[1:]...).CombinedOutput(); err != nil {
				t.Skipf("mkfs failed: %v %s", err, out)
			}
			mnt := filepath.Join(dir, "mnt")
			if err := os.Mkdir(mnt, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if out, err := exec.Command("mount", loop, mnt).CombinedOutput(); err != nil {
				t.Skipf("mount failed: %v %s", err, out)
			}
			t.Cleanup(func() { exec.Command("umount", mnt).Run() })
			path := filepath.Join(mnt, "file")
			dest, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
			if err != nil {
				t.Fatalf("open dest: %v", err)
			}
			defer dest.Close()
			bs := 4096
			if _, err := dest.Write(bytes.Repeat([]byte{1}, bs)); err != nil {
				t.Fatalf("write prefix: %v", err)
			}
			cfg := &config.Config{BlockSize: bs, Sparse: "never"}
			if err := writeZeroBlock(cfg, dest, uint64(bs), zap.NewNop()); err != nil {
				t.Fatalf("writeZeroBlock: %v", err)
			}
			off, err := unix.Seek(int(dest.Fd()), int64(bs), unix.SEEK_DATA)
			if err != nil {
				t.Fatalf("seek data: %v", err)
			}
			if off != int64(bs) {
				t.Fatalf("expected data at %d got %d", bs, off)
			}
			buf := make([]byte, bs)
			if _, err := dest.ReadAt(buf, int64(bs)); err != nil {
				t.Fatalf("read: %v", err)
			}
			if !bytes.Equal(buf, make([]byte, bs)) {
				t.Fatalf("expected zero block")
			}
		})
	}
}
