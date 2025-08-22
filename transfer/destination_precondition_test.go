package transfer

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	rootcmd "github.com/oferchen/lvmsync_go/cmd/root"
	"github.com/oferchen/lvmsync_go/device"
	"github.com/oferchen/lvmsync_go/internal/config"
	"github.com/oferchen/lvmsync_go/internal/exitcode"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/sys/unix"
	manifestpkg "github.com/oferchen/lvmsync_go/manifest"
)

func TestProcessDumpDataDeviceIDMismatchPrecondition(t *testing.T) {
	stream := minimalStream(t)
	for _, force := range []bool{false, true} {
		t.Run("force_"+strconv.FormatBool(force), func(t *testing.T) {
			info := device.NewInfoWithDeps(
				func(context.Context, string) (string, error) { return "actual", nil },
				func(context.Context, string) (string, error) { return "", errors.New("no lvm") },
				func(context.Context, string) (bool, error) { return false, nil },
				nil,
				nil,
			)
			core, logs := observer.New(zap.ErrorLevel)
			tr := NewTransfer(zap.New(core), &sync.WaitGroup{}, info)
			cfg := &config.Config{BlockSize: 1024, Compress: "none", MaxRetries: 1, DeviceUUID: "expected", DedupStrategy: "none", VerifyChecksum: true, ChecksumAlgorithm: "sha256"}
			if force {
				cfg.Force = true
			}
			dest := filepath.Join(t.TempDir(), "dest")
			if err := os.WriteFile(dest, make([]byte, 1024), 0o600); err != nil {
				t.Fatalf("write dest: %v", err)
			}
			err := tr.ProcessDumpData(context.Background(), cfg, bytes.NewReader(stream), dest)
			if err == nil {
				t.Fatalf("expected error for device mismatch")
			}
			if rootcmd.ExitCode(err) != exitcode.ErrPrecondition {
				t.Fatalf("expected precondition exit code, got %d: %v", rootcmd.ExitCode(err), err)
			}
			if logs.FilterMessage("device_id_mismatch").Len() == 0 {
				t.Fatalf("expected device_id_mismatch log")
			}
		})
	}
}

func TestProcessDumpDataSizeMismatchPrecondition(t *testing.T) {
	stream := minimalStream(t)
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.WriteFile(dest, make([]byte, 1024), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	man := filepath.Join(t.TempDir(), "man")
	var hdr manifestpkg.Header
	hdr.Version = manifestpkg.Version
	hdr.BlockSize = 512
	hdr.SizeBytes = 2048
	hdr.ChunkCount = 2
	var st unix.Stat_t
	if err := unix.Stat(dest, &st); err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	hdr.Major = uint32(unix.Major(uint64(st.Rdev)))
	hdr.Minor = uint32(unix.Minor(uint64(st.Rdev)))
	copy(hdr.DeviceID[:], []byte("id"))
	hdr.MAC = manifestHeaderMAC(&hdr)
	f, err := os.Create(man)
	if err != nil {
		t.Fatalf("create manifest: %v", err)
	}
	if err := binary.Write(f, binary.LittleEndian, &hdr); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	f.Close()
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "id", nil },
		func(context.Context, string) (string, error) { return "", errors.New("no lvm") },
		func(context.Context, string) (bool, error) { return false, nil },
		nil,
		nil,
	)
	core, logs := observer.New(zap.ErrorLevel)
	tr := NewTransfer(zap.New(core), &sync.WaitGroup{}, info)
	cfg := &config.Config{BlockSize: 1024, Compress: "none", MaxRetries: 1, DedupStrategy: "none", VerifyChecksum: true, ChecksumAlgorithm: "sha256", ManifestPath: man}
	err = tr.ProcessDumpData(context.Background(), cfg, bytes.NewReader(stream), dest)
	if err == nil {
		t.Fatalf("expected size mismatch error")
	}
	if rootcmd.ExitCode(err) != exitcode.ErrPrecondition {
		t.Fatalf("expected precondition exit code, got %d: %v", rootcmd.ExitCode(err), err)
	}
	if logs.FilterMessage("device_size_mismatch").Len() == 0 {
		t.Fatalf("expected device_size_mismatch log")
	}
}
