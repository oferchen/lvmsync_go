//go:build integration

package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// runCmd executes the command and fails the test on error, returning trimmed output.
func runCmd(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v failed: %v\n%s", cmd.Args, err, out)
	}
	return string(bytes.TrimSpace(out))
}

func TestHolePunchStBlocks(t *testing.T) {
	for _, tool := range []string{"losetup", "mkfs.ext4", "mount", "umount"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}

	tmpDir := t.TempDir()
	tmpfsDir := filepath.Join(tmpDir, "tmpfs")
	if err := os.Mkdir(tmpfsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	runCmd(t, exec.Command("mount", "-t", "tmpfs", "tmpfs", tmpfsDir))
	defer exec.Command("umount", tmpfsDir).Run()

	img := filepath.Join(tmpfsDir, "disk.img")
	fimg, err := os.Create(img)
	if err != nil {
		t.Fatalf("create img: %v", err)
	}
	if err := fimg.Truncate(8 << 20); err != nil { // 8 MiB
		t.Fatalf("truncate img: %v", err)
	}
	if err := fimg.Close(); err != nil {
		t.Fatalf("close img: %v", err)
	}

	loop := runCmd(t, exec.Command("losetup", "--find", "--show", img))
	defer exec.Command("losetup", "-d", loop).Run()

	runCmd(t, exec.Command("mkfs.ext4", "-q", loop))

	fsMount := filepath.Join(tmpDir, "mnt")
	if err := os.Mkdir(fsMount, 0o755); err != nil {
		t.Fatalf("mkdir fs: %v", err)
	}
	runCmd(t, exec.Command("mount", loop, fsMount))
	defer exec.Command("umount", fsMount).Run()

	filePath := filepath.Join(fsMount, "sparse")
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	defer f.Close()

	if err := f.Truncate(1 << 20); err != nil { // 1 MiB sparse file
		t.Fatalf("truncate: %v", err)
	}

	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		t.Fatalf("fstat initial: %v", err)
	}
	if st.Blocks != 0 {
		t.Fatalf("expected 0 blocks, got %d", st.Blocks)
	}

	if _, err := f.WriteAt(bytes.Repeat([]byte{0xFF}, 4096), 0); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		t.Fatalf("fstat write: %v", err)
	}
	used := st.Blocks
	if used == 0 {
		t.Fatalf("expected blocks after write")
	}

	if err := unix.Fallocate(int(f.Fd()), unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE, 0, 4096); err != nil {
		t.Fatalf("punch hole: %v", err)
	}
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		t.Fatalf("fstat punch: %v", err)
	}
	if st.Blocks >= used {
		t.Fatalf("st_blocks did not decrease: before %d after %d", used, st.Blocks)
	}
}
