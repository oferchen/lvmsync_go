package transfer

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/device"
	hashutil "lvmsync_go/hash"
	"lvmsync_go/internal/config"
	manifestpkg "lvmsync_go/manifest"
	"lvmsync_go/transport"
)

func setupLoop(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("losetup", "--find", "--show", path).CombinedOutput()
	if err != nil {
		t.Skipf("losetup %s: %v: %s", path, err, out)
		return ""
	}
	loop := strings.TrimSpace(string(out))
	t.Cleanup(func() { _ = exec.Command("losetup", "-d", loop).Run() })
	return loop
}

func compareFiles(t *testing.T, a, b string) {
	t.Helper()
	dataA, err := os.ReadFile(a)
	if err != nil {
		t.Fatalf("read %s: %v", a, err)
	}
	dataB, err := os.ReadFile(b)
	if err != nil {
		t.Fatalf("read %s: %v", b, err)
	}
	if !bytes.Equal(dataA, dataB) {
		t.Fatalf("file data mismatch")
	}
}

// TestLoopbackLVMToRawOverSSH streams an LVM snapshot to a raw device over the SSH transport.
func TestLoopbackLVMToRawOverSSH(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	prev := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "id", nil })
	defer device.SetUUIDFunc(prev)

	blockSize := int64(4096)
	snapshot := "vg-lv"
	_, srcFile := createVolumeFiles(t, snapshot, blockSize, []int{0, 1})

	srcLoop := setupLoop(t, srcFile)

	destPath := filepath.Join(t.TempDir(), "dest.img")
	df, err := os.Create(destPath)
	if err != nil {
		t.Fatalf("create dest: %v", err)
	}
	srcData0 := bytes.Repeat([]byte{1}, int(blockSize))
	if _, err := df.Write(srcData0); err != nil {
		t.Fatalf("write dest block0: %v", err)
	}
	if err := df.Truncate(2 * blockSize); err != nil {
		t.Fatalf("truncate dest: %v", err)
	}
	if err := df.Close(); err != nil {
		t.Fatalf("close dest: %v", err)
	}
	destLoop := setupLoop(t, destPath)

	manifestPath := filepath.Join(t.TempDir(), "src.man")
	ctxMan, cancelMan := context.WithTimeout(context.Background(), time.Second)
	defer cancelMan()
	if err := manifestpkg.Rebuild(ctxMan, srcLoop, manifestPath, zap.NewNop(), 0, false, 0, 0, 0, 0); err != nil {
		t.Fatalf("rebuild manifest: %v", err)
	}

	resumePath := filepath.Join(t.TempDir(), "resume.state")
	cfg := &config.Config{
		BlockSize:         int(blockSize),
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		Transport:         "ssh",
		ManifestPath:      manifestPath,
		ResumeState:       resumePath,
		SyncIntervalBytes: 512,
		CheckpointBytes:   1,
	}
	digest0 := blake3.Sum256(srcData0)
	writeResumeState(cfg, zap.NewNop(), resumePath, resumeChunks{Fixed: resumeChunk{Chunk: digest0, Offset: 0, Length: uint32(blockSize)}})

	ctx := context.Background()
	tr, err := transport.Get("ssh", transport.Config{Logger: zap.NewNop(), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true})
	if err != nil {
		t.Fatalf("get transport: %v", err)
	}
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan error)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		applyCfg := &config.Config{
			BlockSize:         int(blockSize),
			Compress:          "none",
			ChecksumAlgorithm: "blake3",
			SyncIntervalBytes: 512,
		}
		tt := NewTransfer(zap.NewNop(), nil)
		done <- tt.ProcessDumpData(context.Background(), applyCfg, conn, destLoop)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	tt := NewTransfer(zap.NewNop(), nil)
	if err := tt.DumpChangesParallel(cfg, snapshot, srcLoop, conn); err != nil {
		t.Fatalf("DumpChangesParallel: %v", err)
	}
	conn.Close()
	if err := <-done; err != nil {
		t.Fatalf("apply: %v", err)
	}

	compareFiles(t, srcFile, destPath)

	idx, err := manifestpkg.Open(manifestPath)
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer idx.Close()
	xx0 := hashutil.SumXXH3(srcData0)
	if !idx.Match(0, uint32(blockSize), 0, xx0, func() [32]byte { return digest0 }) {
		t.Fatalf("manifest missing block0")
	}
	srcData1 := bytes.Repeat([]byte{2}, int(blockSize))
	digest1 := blake3.Sum256(srcData1)
	xx1 := hashutil.SumXXH3(srcData1)
	if !idx.Match(uint64(blockSize), uint32(blockSize), 0, xx1, func() [32]byte { return digest1 }) {
		t.Fatalf("manifest missing block1")
	}
	if _, err := os.Stat(resumePath); !os.IsNotExist(err) {
		t.Fatalf("resume state not cleaned up")
	}
}

// TestLoopbackFileToFileOverTCPTLS streams a file image to another over the TCP+TLS transport.
func TestLoopbackFileToFileOverTCPTLS(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	prev := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "id", nil })
	defer device.SetUUIDFunc(prev)

	blockSize := int64(4096)
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.img")
	dstPath := filepath.Join(dir, "dst.img")

	srcData0 := make([]byte, blockSize)
	srcData1 := make([]byte, blockSize)
	rand.Read(srcData0)
	rand.Read(srcData1)
	if err := os.WriteFile(srcPath, append(srcData0, srcData1...), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dstPath, append(srcData0, make([]byte, blockSize)...), 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	srcLoop := setupLoop(t, srcPath)
	dstLoop := setupLoop(t, dstPath)

	manifestPath := filepath.Join(dir, "src.man")
	ctxMan, cancelMan := context.WithTimeout(context.Background(), time.Second)
	defer cancelMan()
	if err := manifestpkg.Rebuild(ctxMan, srcLoop, manifestPath, zap.NewNop(), 0, false, 0, 0, 0, 0); err != nil {
		t.Fatalf("rebuild manifest: %v", err)
	}

	resumePath := filepath.Join(dir, "resume.state")
	cfg := &config.Config{
		BlockSize:         int(blockSize),
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		Transport:         "tcp+tls",
		ManifestPath:      manifestPath,
		ResumeState:       resumePath,
		SyncIntervalBytes: 512,
		CheckpointBytes:   1,
	}
	digest0 := blake3.Sum256(srcData0)
	writeResumeState(cfg, zap.NewNop(), resumePath, resumeChunks{Fixed: resumeChunk{Chunk: digest0, Offset: 0, Length: uint32(blockSize)}})

	ctx := context.Background()
	tr, err := transport.Get("tcp+tls", transport.Config{Logger: zap.NewNop(), AllowInsecure: true})
	if err != nil {
		t.Fatalf("get transport: %v", err)
	}
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan error)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		applyCfg := &config.Config{
			BlockSize:         int(blockSize),
			Compress:          "none",
			ChecksumAlgorithm: "blake3",
			SyncIntervalBytes: 512,
		}
		tt := NewTransfer(zap.NewNop(), nil)
		done <- tt.ProcessDumpData(context.Background(), applyCfg, conn, dstLoop)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	tt := NewTransfer(zap.NewNop(), nil)

	ranges := []Range{{Start: 0, End: uint64(blockSize)}, {Start: uint64(blockSize), End: uint64(2 * blockSize)}}
	compWriter, bufOut, err := setupOutput(cfg, conn, "", zap.NewNop())
	if err != nil {
		t.Fatalf("setupOutput: %v", err)
	}
	srcFile, err := os.Open(srcLoop)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	checkpoint := readResumeState(cfg, tt.Logger)
	startIdx := findResumeIndex(cfg, srcFile, ranges, checkpoint, tt.Logger)
	if startIdx > 0 {
		ranges = ranges[startIdx:]
	}
	if _, _, _, err := iterateBlocks(cfg, ranges, srcFile, bufOut, nil, [2]int{-1, -1}, tt.Logger, tt.Tracker); err != nil {
		t.Fatalf("iterateBlocks: %v", err)
	}
	finalizeProgress(cfg, tt.Logger)
	cleanupOutput(bufOut, compWriter, tt.Logger)
	finalizeResumeState(cfg, tt.Tracker, tt.Logger)
	srcFile.Close()
	conn.Close()
	if err := <-done; err != nil {
		t.Fatalf("apply: %v", err)
	}

	compareFiles(t, srcPath, dstPath)
	idx, err := manifestpkg.Open(manifestPath)
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer idx.Close()
	xx0 := hashutil.SumXXH3(srcData0)
	if !idx.Match(0, uint32(blockSize), 0, xx0, func() [32]byte { return digest0 }) {
		t.Fatalf("manifest missing block0")
	}
	digest1 := blake3.Sum256(srcData1)
	xx1 := hashutil.SumXXH3(srcData1)
	if !idx.Match(uint64(blockSize), uint32(blockSize), 0, xx1, func() [32]byte { return digest1 }) {
		t.Fatalf("manifest missing block1")
	}
	if _, err := os.Stat(resumePath); !os.IsNotExist(err) {
		t.Fatalf("resume state not cleaned up")
	}
}

// TestLoopbackLVMToRawOverTCPTLS streams an LVM snapshot to a raw device over the TCP+TLS transport.
func TestLoopbackLVMToRawOverTCPTLS(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	prev := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "id", nil })
	defer device.SetUUIDFunc(prev)

	for _, mode := range []string{"fixed", "cdc", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			blockSize := int64(4096)
			snapshot := "vg-lv"
			_, srcFile := createVolumeFiles(t, snapshot, blockSize, []int{0, 1})

			srcLoop := setupLoop(t, srcFile)

			destPath := filepath.Join(t.TempDir(), "dest.img")
			df, err := os.Create(destPath)
			if err != nil {
				t.Fatalf("create dest: %v", err)
			}
			srcData0 := bytes.Repeat([]byte{1}, int(blockSize))
			if _, err := df.Write(srcData0); err != nil {
				t.Fatalf("write dest block0: %v", err)
			}
			if err := df.Truncate(2 * blockSize); err != nil {
				t.Fatalf("truncate dest: %v", err)
			}
			if err := df.Close(); err != nil {
				t.Fatalf("close dest: %v", err)
			}
			destLoop := setupLoop(t, destPath)

			manifestPath := filepath.Join(t.TempDir(), "src.man")
			ctxMan, cancelMan := context.WithTimeout(context.Background(), time.Second)
			defer cancelMan()
			if err := manifestpkg.Rebuild(ctxMan, srcLoop, manifestPath, zap.NewNop(), 0, false, 0, 0, 0, 0); err != nil {
				t.Fatalf("rebuild manifest: %v", err)
			}

			resumePath := filepath.Join(t.TempDir(), "resume.state")
			dedupState := filepath.Join(t.TempDir(), "dedup.state")
			cfg := &config.Config{
				BlockSize:         int(blockSize),
				Compress:          "auto",
				ChecksumAlgorithm: "blake3",
				Transport:         "tcp+tls",
				ManifestPath:      manifestPath,
				ResumeState:       resumePath,
				DedupMode:         mode,
				DedupStrategy:     "checksum",
				DedupStateFile:    dedupState,
				ODirect:           true,
				SyncIntervalBytes: 512,
				CheckpointBytes:   1,
			}
			digest0 := blake3.Sum256(srcData0)
			writeResumeState(cfg, zap.NewNop(), resumePath, resumeChunks{Fixed: resumeChunk{Chunk: digest0, Offset: 0, Length: uint32(blockSize)}})

			ctx := context.Background()
			tr, err := transport.Get("tcp+tls", transport.Config{Logger: zap.NewNop(), AllowInsecure: true})
			if err != nil {
				t.Fatalf("get transport: %v", err)
			}
			ln, err := tr.Listen(ctx, "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			defer ln.Close()

			done := make(chan error)
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				applyCfg := &config.Config{
					BlockSize:         int(blockSize),
					Compress:          "auto",
					ChecksumAlgorithm: "blake3",
					DedupMode:         mode,
					DedupStrategy:     "checksum",
					DedupStateFile:    filepath.Join(t.TempDir(), "apply_dedup.state"),
					ODirect:           true,
					SyncIntervalBytes: 512,
				}
				tt := NewTransfer(zap.NewNop(), nil)
				done <- tt.ProcessDumpData(context.Background(), applyCfg, conn, destLoop)
			}()

			conn, err := tr.Dial(ctx, ln.Addr().String())
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			tt := NewTransfer(zap.NewNop(), nil)
			if err := tt.DumpChangesParallel(cfg, snapshot, srcLoop, conn); err != nil {
				t.Fatalf("DumpChangesParallel: %v", err)
			}
			conn.Close()
			if err := <-done; err != nil {
				t.Fatalf("apply: %v", err)
			}

			compareFiles(t, srcFile, destPath)

			idx, err := manifestpkg.Open(manifestPath)
			if err != nil {
				t.Fatalf("open manifest: %v", err)
			}
			defer idx.Close()
			xx0 := hashutil.SumXXH3(srcData0)
			if !idx.Match(0, uint32(blockSize), 0, xx0, func() [32]byte { return digest0 }) {
				t.Fatalf("manifest missing block0")
			}
			srcData1 := bytes.Repeat([]byte{2}, int(blockSize))
			digest1 := blake3.Sum256(srcData1)
			xx1 := hashutil.SumXXH3(srcData1)
			if !idx.Match(uint64(blockSize), uint32(blockSize), 0, xx1, func() [32]byte { return digest1 }) {
				t.Fatalf("manifest missing block1")
			}
			if _, err := os.Stat(resumePath); !os.IsNotExist(err) {
				t.Fatalf("resume state not cleaned up")
			}
			if _, err := os.Stat(dedupState); err != nil {
				t.Fatalf("dedup state not saved: %v", err)
			}
		})
	}
}

func runRawToRawLoopback(t *testing.T, transportName string) {
	for _, mode := range []string{"fixed", "cdc", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			blockSize := int64(4096)
			dir := t.TempDir()
			srcPath := filepath.Join(dir, "src.img")
			dstPath := filepath.Join(dir, "dst.img")

			srcData0 := make([]byte, blockSize)
			srcData1 := make([]byte, blockSize)
			rand.Read(srcData0)
			rand.Read(srcData1)
			if err := os.WriteFile(srcPath, append(srcData0, srcData1...), 0o600); err != nil {
				t.Fatalf("write src: %v", err)
			}
			if err := os.WriteFile(dstPath, append(srcData0, make([]byte, blockSize)...), 0o600); err != nil {
				t.Fatalf("write dst: %v", err)
			}

			srcLoop := setupLoop(t, srcPath)
			dstLoop := setupLoop(t, dstPath)

			manifestPath := filepath.Join(dir, "src.man")
			ctxMan, cancelMan := context.WithTimeout(context.Background(), time.Second)
			defer cancelMan()
			if err := manifestpkg.Rebuild(ctxMan, srcLoop, manifestPath, zap.NewNop(), 0, false, 0, 0, 0, 0); err != nil {
				t.Fatalf("rebuild manifest: %v", err)
			}

			resumePath := filepath.Join(dir, "resume.state")
			dedupState := filepath.Join(dir, "dedup.state")
			cfg := &config.Config{
				BlockSize:         int(blockSize),
				Compress:          "auto",
				ChecksumAlgorithm: "blake3",
				Transport:         transportName,
				ManifestPath:      manifestPath,
				ResumeState:       resumePath,
				DedupMode:         mode,
				DedupStrategy:     "checksum",
				DedupStateFile:    dedupState,
				ODirect:           true,
				SyncIntervalBytes: 512,
				CheckpointBytes:   1,
			}
			digest0 := blake3.Sum256(srcData0)
			writeResumeState(cfg, zap.NewNop(), resumePath, resumeChunks{Fixed: resumeChunk{Chunk: digest0, Offset: 0, Length: uint32(blockSize)}})

			ctx := context.Background()
			tcfg := transport.Config{Logger: zap.NewNop()}
			if transportName == "tcp+tls" {
				tcfg.AllowInsecure = true
			}
			tr, err := transport.Get(transportName, tcfg)
			if err != nil {
				t.Fatalf("get transport: %v", err)
			}
			ln, err := tr.Listen(ctx, "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			defer ln.Close()

			done := make(chan error)
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				applyCfg := &config.Config{
					BlockSize:         int(blockSize),
					Compress:          "auto",
					ChecksumAlgorithm: "blake3",
					DedupMode:         mode,
					DedupStrategy:     "checksum",
					DedupStateFile:    filepath.Join(t.TempDir(), "apply_dedup.state"),
					ODirect:           true,
					SyncIntervalBytes: 512,
				}
				tt := NewTransfer(zap.NewNop(), nil)
				done <- tt.ProcessDumpData(context.Background(), applyCfg, conn, dstLoop)
			}()

			conn, err := tr.Dial(ctx, ln.Addr().String())
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			tt := NewTransfer(zap.NewNop(), nil)

			ranges := []Range{{Start: 0, End: uint64(blockSize)}, {Start: uint64(blockSize), End: uint64(2 * blockSize)}}
			compWriter, bufOut, err := setupOutput(cfg, conn, "", zap.NewNop())
			if err != nil {
				t.Fatalf("setupOutput: %v", err)
			}
			srcFile, err := os.Open(srcLoop)
			if err != nil {
				t.Fatalf("open src: %v", err)
			}
			checkpoint := readResumeState(cfg, tt.Logger)
			startIdx := findResumeIndex(cfg, srcFile, ranges, checkpoint, tt.Logger)
			if startIdx > 0 {
				ranges = ranges[startIdx:]
			}
			dedup, cleanup := tt.setupDedup(cfg)
			if _, _, _, err := iterateBlocks(cfg, ranges, srcFile, bufOut, dedup, [2]int{-1, -1}, tt.Logger, tt.Tracker); err != nil {
				t.Fatalf("iterateBlocks: %v", err)
			}
			cleanup()
			finalizeProgress(cfg, tt.Logger)
			cleanupOutput(bufOut, compWriter, tt.Logger)
			finalizeResumeState(cfg, tt.Tracker, tt.Logger)
			srcFile.Close()
			conn.Close()
			if err := <-done; err != nil {
				t.Fatalf("apply: %v", err)
			}

			compareFiles(t, srcPath, dstPath)
			idx, err := manifestpkg.Open(manifestPath)
			if err != nil {
				t.Fatalf("open manifest: %v", err)
			}
			defer idx.Close()
			xx0 := hashutil.SumXXH3(srcData0)
			if !idx.Match(0, uint32(blockSize), 0, xx0, func() [32]byte { return digest0 }) {
				t.Fatalf("manifest missing block0")
			}
			digest1 := blake3.Sum256(srcData1)
			xx1 := hashutil.SumXXH3(srcData1)
			if !idx.Match(uint64(blockSize), uint32(blockSize), 0, xx1, func() [32]byte { return digest1 }) {
				t.Fatalf("manifest missing block1")
			}
			if _, err := os.Stat(resumePath); !os.IsNotExist(err) {
				t.Fatalf("resume state not cleaned up")
			}
			if _, err := os.Stat(dedupState); err != nil {
				t.Fatalf("dedup state not saved: %v", err)
			}
		})
	}
}

// TestLoopbackRawToRawOverSSH streams a raw device to another over the SSH transport.
func TestLoopbackRawToRawOverSSH(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	runRawToRawLoopback(t, "ssh")
}

// TestLoopbackRawToRawOverTCPTLS streams a raw device to another over the TCP+TLS transport.
func TestLoopbackRawToRawOverTCPTLS(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	runRawToRawLoopback(t, "tcp+tls")
}

// TestLoopbackSparseFileToFileOverSSH streams a sparse file image to another over the SSH transport.
func TestLoopbackSparseFileToFileOverSSH(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	prev := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "id", nil })
	defer device.SetUUIDFunc(prev)

	for _, mode := range []string{"fixed", "cdc", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			blockSize := int64(4096)
			dir := t.TempDir()
			srcPath := filepath.Join(dir, "src.img")
			dstPath := filepath.Join(dir, "dst.img")

			srcData0 := make([]byte, blockSize)
			srcData1 := make([]byte, blockSize)
			rand.Read(srcData0)
			rand.Read(srcData1)
			if err := os.WriteFile(srcPath, append(srcData0, srcData1...), 0o600); err != nil {
				t.Fatalf("write src: %v", err)
			}
			df, err := os.Create(dstPath)
			if err != nil {
				t.Fatalf("create dst: %v", err)
			}
			if _, err := df.Write(srcData0); err != nil {
				t.Fatalf("write dst block0: %v", err)
			}
			if err := df.Truncate(2 * blockSize); err != nil {
				t.Fatalf("truncate dst: %v", err)
			}
			if err := df.Close(); err != nil {
				t.Fatalf("close dst: %v", err)
			}

			srcLoop := setupLoop(t, srcPath)
			dstLoop := setupLoop(t, dstPath)

			manifestPath := filepath.Join(dir, "src.man")
			ctxMan, cancelMan := context.WithTimeout(context.Background(), time.Second)
			defer cancelMan()
			if err := manifestpkg.Rebuild(ctxMan, srcLoop, manifestPath, zap.NewNop(), 0, false, 0, 0, 0, 0); err != nil {
				t.Fatalf("rebuild manifest: %v", err)
			}

			resumePath := filepath.Join(dir, "resume.state")
			dedupState := filepath.Join(dir, "dedup.state")
			cfg := &config.Config{
				BlockSize:         int(blockSize),
				Compress:          "auto",
				ChecksumAlgorithm: "blake3",
				Transport:         "ssh",
				ManifestPath:      manifestPath,
				ResumeState:       resumePath,
				DedupMode:         mode,
				DedupStrategy:     "checksum",
				DedupStateFile:    dedupState,
				ODirect:           true,
				SyncIntervalBytes: 512,
				CheckpointBytes:   1,
			}
			digest0 := blake3.Sum256(srcData0)
			writeResumeState(cfg, zap.NewNop(), resumePath, resumeChunks{Fixed: resumeChunk{Chunk: digest0, Offset: 0, Length: uint32(blockSize)}})

			ctx := context.Background()
			tr, err := transport.Get("ssh", transport.Config{Logger: zap.NewNop(), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true})
			if err != nil {
				t.Fatalf("get transport: %v", err)
			}
			ln, err := tr.Listen(ctx, "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			defer ln.Close()

			done := make(chan error)
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				applyCfg := &config.Config{
					BlockSize:         int(blockSize),
					Compress:          "auto",
					ChecksumAlgorithm: "blake3",
					DedupMode:         mode,
					DedupStrategy:     "checksum",
					DedupStateFile:    filepath.Join(t.TempDir(), "apply_dedup.state"),
					ODirect:           true,
					SyncIntervalBytes: 512,
				}
				tt := NewTransfer(zap.NewNop(), nil)
				done <- tt.ProcessDumpData(context.Background(), applyCfg, conn, dstLoop)
			}()

			conn, err := tr.Dial(ctx, ln.Addr().String())
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			tt := NewTransfer(zap.NewNop(), nil)

			ranges := []Range{{Start: 0, End: uint64(blockSize)}, {Start: uint64(blockSize), End: uint64(2 * blockSize)}}
			compWriter, bufOut, err := setupOutput(cfg, conn, "", zap.NewNop())
			if err != nil {
				t.Fatalf("setupOutput: %v", err)
			}
			srcFile, err := os.Open(srcLoop)
			if err != nil {
				t.Fatalf("open src: %v", err)
			}
			checkpoint := readResumeState(cfg, tt.Logger)
			startIdx := findResumeIndex(cfg, srcFile, ranges, checkpoint, tt.Logger)
			if startIdx > 0 {
				ranges = ranges[startIdx:]
			}
			dedup, cleanup := tt.setupDedup(cfg)
			if _, _, _, err := iterateBlocks(cfg, ranges, srcFile, bufOut, dedup, [2]int{-1, -1}, tt.Logger, tt.Tracker); err != nil {
				t.Fatalf("iterateBlocks: %v", err)
			}
			cleanup()
			finalizeProgress(cfg, tt.Logger)
			cleanupOutput(bufOut, compWriter, tt.Logger)
			finalizeResumeState(cfg, tt.Tracker, tt.Logger)
			srcFile.Close()
			conn.Close()
			if err := <-done; err != nil {
				t.Fatalf("apply: %v", err)
			}

			compareFiles(t, srcPath, dstPath)
			idx, err := manifestpkg.Open(manifestPath)
			if err != nil {
				t.Fatalf("open manifest: %v", err)
			}
			defer idx.Close()
			xx0 := hashutil.SumXXH3(srcData0)
			if !idx.Match(0, uint32(blockSize), 0, xx0, func() [32]byte { return digest0 }) {
				t.Fatalf("manifest missing block0")
			}
			digest1 := blake3.Sum256(srcData1)
			xx1 := hashutil.SumXXH3(srcData1)
			if !idx.Match(uint64(blockSize), uint32(blockSize), 0, xx1, func() [32]byte { return digest1 }) {
				t.Fatalf("manifest missing block1")
			}
			if _, err := os.Stat(resumePath); !os.IsNotExist(err) {
				t.Fatalf("resume state not cleaned up")
			}
			if _, err := os.Stat(dedupState); err != nil {
				t.Fatalf("dedup state not saved: %v", err)
			}
		})
	}
}
