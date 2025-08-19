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
	"testing"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	privilege "lvmsync_go/internal/privilege"
	"lvmsync_go/transport"
)

// waitForBlocks waits until at least blocks blocks have been written to dest.
func waitForBlocks(t *testing.T, dest string, blockSize, blocks int) {
	if blocks == 0 {
		return
	}
	expected := bytes.Repeat([]byte{byte(blocks)}, blockSize)
	offset := int64(blockSize * (blocks - 1))
	for i := 0; i < 100; i++ {
		f, err := os.Open(dest)
		if err == nil {
			data := make([]byte, blockSize)
			f.ReadAt(data, offset)
			f.Close()
			if bytes.Equal(data, expected) {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d blocks", blocks)
}

// throttleCopy copies from src to dst in small chunks with delays to slow progress.
func throttleCopy(dst io.Writer, src io.Reader) {
	buf := make([]byte, 1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			dst.Write(buf[:n])
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			return
		}
	}
}

// TestResumeAfterKill verifies resume works when the apply process is killed at various progress percentages.
func TestResumeAfterKill(t *testing.T) {
	blockSize := 4096
	totalBlocks := 4
	snapshot := "vg-lv"
	_, src := createVolumeFiles(t, snapshot, int64(blockSize), []int{0, 1, 2, 3})

	detect := func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *device.Runner) (device.Device, error) {
		return &mockDevice{path: src, size: uint64(blockSize * totalBlocks), blockSize: uint64(blockSize)}, nil
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

	for _, pct := range []int{25, 50, 75} {
		t.Run(fmt.Sprintf("pct%d", pct), func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "dest")
			if err := os.WriteFile(dest, make([]byte, blockSize*totalBlocks), 0o600); err != nil {
				t.Fatalf("write dest: %v", err)
			}
			resume := filepath.Join(t.TempDir(), "resume.state")

			cmd := exec.Command(os.Args[0], "-test.run=TestApplyHelper", "--")
			cmd.Env = append(os.Environ(), "APPLY_HELPER=1", fmt.Sprintf("DEST=%s", dest), fmt.Sprintf("RESUME=%s", resume), fmt.Sprintf("BLOCKSIZE=%d", blockSize))
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatalf("stdin: %v", err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatalf("start: %v", err)
			}
			pr := bytes.NewReader(buf.Bytes())
			go func() {
				throttleCopy(stdin, pr)
				stdin.Close()
			}()

			blocks := pct * totalBlocks / 100
			waitForBlocks(t, dest, blockSize, blocks)
			cmd.Process.Kill()
			cmd.Wait()

			cmd2 := exec.Command(os.Args[0], "-test.run=TestApplyHelper", "--")
			cmd2.Env = append(os.Environ(), "APPLY_HELPER=1", fmt.Sprintf("DEST=%s", dest), fmt.Sprintf("RESUME=%s", resume), fmt.Sprintf("BLOCKSIZE=%d", blockSize))
			stdin2, err := cmd2.StdinPipe()
			if err != nil {
				t.Fatalf("stdin2: %v", err)
			}
			if err := cmd2.Start(); err != nil {
				t.Fatalf("start2: %v", err)
			}
			go func() {
				throttleCopy(stdin2, bytes.NewReader(buf.Bytes()))
				stdin2.Close()
			}()
			if err := cmd2.Wait(); err != nil {
				t.Fatalf("resume apply: %v", err)
			}

			srcData, err := os.ReadFile(src)
			if err != nil {
				t.Fatalf("read src: %v", err)
			}
			dstData, err := os.ReadFile(dest)
			if err != nil {
				t.Fatalf("read dest: %v", err)
			}
			if !bytes.Equal(srcData, dstData) {
				t.Fatalf("destination mismatch after resume")
			}
			if blake3.Sum256(srcData) != blake3.Sum256(dstData) {
				t.Fatalf("checksum mismatch")
			}
		})
	}
}

// TestResumeAfterNetworkDrop closes the connection mid-transfer and verifies resume succeeds.
func TestResumeAfterNetworkDrop(t *testing.T) {
	blockSize := 4096
	totalBlocks := 4
	snapshot := "vg-lv"
	_, src := createVolumeFiles(t, snapshot, int64(blockSize), []int{0, 1, 2, 3})
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.WriteFile(dest, make([]byte, blockSize*totalBlocks), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	resume := filepath.Join(t.TempDir(), "resume.state")

	detect := func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *device.Runner) (device.Device, error) {
		return &mockDevice{path: src, size: uint64(blockSize * totalBlocks), blockSize: uint64(blockSize)}, nil
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

	ctx := context.Background()
	trNet, err := transport.Get("tcp+tls", transport.Config{Logger: zap.NewNop(), AllowInsecure: true})
	if err != nil {
		t.Fatalf("get transport: %v", err)
	}

	applyCfg := &config.Config{BlockSize: blockSize, Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: resume, SyncIntervalBytes: 512}

	ln, err := trNet.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	applyDone := make(chan error)
	go func() {
		tt := NewTransfer(zap.NewNop(), nil, nil)
		applyDone <- tt.ProcessDumpData(context.Background(), applyCfg, conn, dest)
	}()

	clientConn, err := trNet.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	go func() {
		throttleCopy(clientConn, bytes.NewReader(buf.Bytes()))
		clientConn.Close()
	}()

	waitForBlocks(t, dest, blockSize, 1)
	conn.Close()
	<-applyDone
	ln.Close()

	ln2, err := trNet.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen2: %v", err)
	}
	connDone := make(chan error)
	go func() {
		conn2, err := ln2.Accept()
		if err != nil {
			connDone <- err
			return
		}
		tt := NewTransfer(zap.NewNop(), nil, nil)
		connDone <- tt.ProcessDumpData(context.Background(), applyCfg, conn2, dest)
	}()
	clientConn2, err := trNet.Dial(ctx, ln2.Addr().String())
	if err != nil {
		t.Fatalf("dial2: %v", err)
	}
	go func() {
		io.Copy(clientConn2, bytes.NewReader(buf.Bytes()))
		clientConn2.Close()
	}()
	if err := <-connDone; err != nil {
		t.Fatalf("resume apply: %v", err)
	}

	srcData, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read src: %v", err)
	}
	dstData, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(srcData, dstData) {
		t.Fatalf("destination mismatch after network resume")
	}
	if blake3.Sum256(srcData) != blake3.Sum256(dstData) {
		t.Fatalf("checksum mismatch")
	}
}
