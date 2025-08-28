//go:build integration

package transfer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"golang.org/x/sys/unix"
	"github.com/oferchen/lvmsync_go/device"
	hashutil "github.com/oferchen/lvmsync_go/hash"
	"github.com/oferchen/lvmsync_go/internal/config"
	privilege "github.com/oferchen/lvmsync_go/internal/privilege"
	manifestpkg "github.com/oferchen/lvmsync_go/manifest"
)

// TestApplyVerifyHelper is a helper process that runs ProcessDumpData reading from stdin.
func TestApplyVerifyHelper(t *testing.T) {
	if os.Getenv("APPLY_VERIFY_HELPER") != "1" {
		return
	}
	dest := os.Getenv("DEST")
	resume := os.Getenv("RESUME")
	bs, _ := strconv.Atoi(os.Getenv("BLOCKSIZE"))
	rv := os.Getenv("RESUME_VERIFY") == "1"
	man := os.Getenv("MANIFEST")

	detect := func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *device.Runner) (device.Device, error) {
		return &mockDevice{path: dest, size: uint64(2 * bs), blockSize: uint64(bs)}, nil
	}
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "uuid", nil },
		nil,
		func(context.Context, string) (bool, error) { return false, nil },
		nil,
		detect,
	)
	tr := NewTransfer(zap.NewNop(), nil, info)
	cfg := &config.Config{BlockSize: bs, Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: resume, MaxRetries: 1}
	if rv {
		cfg.ResumeVerify = true
		cfg.ManifestPath = man
	}
	if err := tr.ProcessDumpData(context.Background(), cfg, os.Stdin, dest); err != nil {
		fmt.Fprintln(os.Stderr, "apply error:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// TestResumeVerifyAndVerifyOnly kills an apply process mid-transfer, resumes with --resume=verify, and verifies the final state.
func TestResumeVerifyAndVerifyOnly(t *testing.T) {
	dir := t.TempDir()
	blockSize := 4096
	snapshot := "vg-lv"
	_, src := createVolumeFiles(t, snapshot, int64(blockSize), []int{0, 1})
	dest := filepath.Join(dir, "dest")
	if err := os.WriteFile(dest, make([]byte, blockSize*2), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	resume := filepath.Join(dir, "resume.state")
	man := filepath.Join(dir, "src.man")
	var st unix.Stat_t
	if err := unix.Stat(src, &st); err != nil {
		t.Fatalf("stat src: %v", err)
	}
	idx, err := manifestpkg.Create(man, "uuid", uint64(2*blockSize), 0, uint32(unix.Major(uint64(st.Rdev))), uint32(unix.Minor(uint64(st.Rdev))), uint32(blockSize), 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	firstBlock := bytes.Repeat([]byte{1}, blockSize)
	dig := blake3.Sum256(firstBlock)
	xx := hashutil.SumXXH3(firstBlock)
	if err := idx.Set(0, uint32(blockSize), 0, xx, dig); err != nil {
		t.Fatalf("manifest set first: %v", err)
	}
	secondBlock := bytes.Repeat([]byte{2}, blockSize)
	dig = blake3.Sum256(secondBlock)
	xx = hashutil.SumXXH3(secondBlock)
	if err := idx.Set(uint64(blockSize), uint32(blockSize), 0, xx, dig); err != nil {
		t.Fatalf("manifest set second: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("manifest close: %v", err)
	}

	// Build dump stream once.
	detect := func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *device.Runner) (device.Device, error) {
		return &mockDevice{path: src, size: uint64(2 * blockSize), blockSize: uint64(blockSize)}, nil
	}
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "uuid", nil },
		nil,
		func(context.Context, string) (bool, error) { return false, nil },
		func(context.Context, string, uint64) ([32]byte, error) {
			first := bytes.Repeat([]byte{1}, blockSize)
			return blake3.Sum256(first), nil
		},
		detect,
	)
	tr := NewTransfer(zap.NewNop(), nil, info)
	var buf bytes.Buffer
	dumpCfg := &config.Config{BlockSize: blockSize, Compress: "none", ChecksumAlgorithm: "blake3", DedupMode: "fixed", MaxRetries: 1}
	if err := tr.DumpChangesParallel(context.Background(), dumpCfg, snapshot, src, &buf); err != nil {
		t.Fatalf("dump: %v", err)
	}

	// Start apply helper and kill mid-transfer.
	cmd := exec.Command(os.Args[0], "-test.run=TestApplyVerifyHelper", "--")
	cmd.Env = append(os.Environ(), "APPLY_VERIFY_HELPER=1", fmt.Sprintf("DEST=%s", dest), fmt.Sprintf("RESUME=%s", resume), fmt.Sprintf("BLOCKSIZE=%d", blockSize))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	go func() {
		io.Copy(stdin, &buf)
		stdin.Close()
	}()

	walPath := resume + ".wal"
	first := firstBlock
	for i := 0; i < 100; i++ {
		fi, err := os.Stat(walPath)
		if err == nil && fi.Size() >= 128 {
			f, _ := os.Open(dest)
			data := make([]byte, blockSize)
			f.ReadAt(data, 0)
			f.Close()
			if bytes.Equal(data, first) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	cmd.Process.Kill()
	cmd.Wait()

	// Resume apply with --resume=verify and manifest.
	cmd2 := exec.Command(os.Args[0], "-test.run=TestApplyVerifyHelper", "--")
	cmd2.Env = append(os.Environ(), "APPLY_VERIFY_HELPER=1", fmt.Sprintf("DEST=%s", dest), fmt.Sprintf("RESUME=%s", resume), fmt.Sprintf("BLOCKSIZE=%d", blockSize), "RESUME_VERIFY=1", fmt.Sprintf("MANIFEST=%s", man))
	stdin2, err := cmd2.StdinPipe()
	if err != nil {
		t.Fatalf("stdin2: %v", err)
	}
	if err := cmd2.Start(); err != nil {
		t.Fatalf("start2: %v", err)
	}
	go func() {
		io.Copy(stdin2, bytes.NewReader(buf.Bytes()))
		stdin2.Close()
	}()
	if err := cmd2.Wait(); err != nil {
		t.Fatalf("resume apply: %v", err)
	}

	// Verify WAL ranges cover entire device without overlap.
	id := device.DeviceIdentity{SizeBytes: uint64(2 * blockSize), KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000000", FSUUID: "uuid"}
	w, ranges, err := OpenWAL(walPath, id, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	w.Close()
	if len(ranges) != 2 || ranges[0].Start != 0 || ranges[0].End != uint64(blockSize) || ranges[1].Start != uint64(blockSize) || ranges[1].End != uint64(2*blockSize) {
		t.Fatalf("unexpected wal ranges %#v", ranges)
	}

	// Final verify-only: ensure destination matches manifest using WAL ranges.
	cfg := &config.Config{BlockSize: blockSize, ManifestPath: man}
	f, err := os.Open(dest)
	if err != nil {
		t.Fatalf("open dest: %v", err)
	}
	defer f.Close()
	if err := verifyWAL(cfg, f, ranges, zap.NewNop()); err != nil {
		t.Fatalf("verifyWAL: %v", err)
	}
}
