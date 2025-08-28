package transfer

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"github.com/oferchen/lvmsync_go/device"
	"github.com/oferchen/lvmsync_go/internal/config"
	"github.com/oferchen/lvmsync_go/internal/exitcode"
	manifestpkg "github.com/oferchen/lvmsync_go/manifest"
)

func TestVerifyDestinationManifestDigestError(t *testing.T) {
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "id", nil },
		nil,
		func(context.Context, string) (bool, error) { return false, nil },
		func(context.Context, string, uint64) ([32]byte, error) { return [32]byte{}, errors.New("boom") },
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.WriteFile(dest, make([]byte, 1024), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	man := filepath.Join(t.TempDir(), "man")
	var hdr manifestpkg.Header
	hdr.Version = manifestpkg.Version
	hdr.BlockSize = 512
	hdr.SizeBytes = 1024
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
	cfg := &config.Config{ManifestPath: man}
	if _, _, _, err := tr.verifyDestination(context.Background(), cfg, dest); err == nil || !strings.Contains(err.Error(), "read destination digest") {
		t.Fatalf("expected digest read error, got %v", err)
	}
}

func TestVerifyDestinationFirstBlockDigestMismatch(t *testing.T) {
	info := device.NewInfoWithDeps(
		nil,
		nil,
		func(context.Context, string) (bool, error) { return false, nil },
		func(context.Context, string, uint64) ([32]byte, error) {
			var d [32]byte
			d[0] = 1
			return d, nil
		},
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.WriteFile(dest, make([]byte, 512), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	cfg := &config.Config{FirstBlockDigest: strings.Repeat("00", 32)}
	if _, _, _, err := tr.verifyDestination(context.Background(), cfg, dest); err == nil || !errors.Is(err, exitcode.ErrVerify) {
		t.Fatalf("expected verify error, got %v", err)
	}
}

func TestVerifyDestinationDeviceNumberMismatch(t *testing.T) {
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "id", nil },
		nil,
		func(context.Context, string) (bool, error) { return false, nil },
		nil,
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.WriteFile(dest, make([]byte, 1024), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	man := filepath.Join(t.TempDir(), "man")
	var hdr manifestpkg.Header
	hdr.Version = manifestpkg.Version
	hdr.BlockSize = 512
	hdr.SizeBytes = 1024
	hdr.ChunkCount = 2
	hdr.Major = 1
	hdr.Minor = 1
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
	cfg := &config.Config{ManifestPath: man}
	if _, _, _, err := tr.verifyDestination(context.Background(), cfg, dest); err == nil || !errors.Is(err, exitcode.ErrPrecondition) {
		t.Fatalf("expected precondition error, got %v", err)
	}
}

func TestVerifyDestinationKernelUUIDMismatch(t *testing.T) {
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "id", nil },
		nil,
		func(context.Context, string) (bool, error) { return false, nil },
		nil,
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.WriteFile(dest, make([]byte, 1024), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	man := filepath.Join(t.TempDir(), "man")
	var hdr manifestpkg.Header
	hdr.Version = manifestpkg.Version
	hdr.BlockSize = 512
	hdr.SizeBytes = 1024
	hdr.ChunkCount = 2
	var st unix.Stat_t
	if err := unix.Stat(dest, &st); err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	hdr.Major = uint32(unix.Major(uint64(st.Rdev)))
	hdr.Minor = uint32(unix.Minor(uint64(st.Rdev)))
	copy(hdr.DeviceID[:], []byte("id"))
	copy(hdr.KernelUUID[:], []byte("kernel"))
	hdr.MAC = manifestHeaderMAC(&hdr)
	f, err := os.Create(man)
	if err != nil {
		t.Fatalf("create manifest: %v", err)
	}
	if err := binary.Write(f, binary.LittleEndian, &hdr); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	f.Close()
	cfg := &config.Config{ManifestPath: man}
	if _, _, _, err := tr.verifyDestination(context.Background(), cfg, dest); err == nil || !errors.Is(err, exitcode.ErrPrecondition) {
		t.Fatalf("expected precondition error, got %v", err)
	}
}

func TestVerifyDestinationPartitionHashMismatch(t *testing.T) {
	info := device.NewInfoWithDeps(
		func(context.Context, string) (string, error) { return "id", nil },
		nil,
		func(context.Context, string) (bool, error) { return false, nil },
		nil,
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.WriteFile(dest, make([]byte, 1024), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	man := filepath.Join(t.TempDir(), "man")
	var hdr manifestpkg.Header
	hdr.Version = manifestpkg.Version
	hdr.BlockSize = 512
	hdr.SizeBytes = 1024
	hdr.ChunkCount = 2
	var st unix.Stat_t
	if err := unix.Stat(dest, &st); err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	hdr.Major = uint32(unix.Major(uint64(st.Rdev)))
	hdr.Minor = uint32(unix.Minor(uint64(st.Rdev)))
	copy(hdr.DeviceID[:], []byte("id"))
	hdr.PartitionHash[0] = 1
	hdr.MAC = manifestHeaderMAC(&hdr)
	f, err := os.Create(man)
	if err != nil {
		t.Fatalf("create manifest: %v", err)
	}
	if err := binary.Write(f, binary.LittleEndian, &hdr); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	f.Close()
	cfg := &config.Config{ManifestPath: man}
	if _, _, _, err := tr.verifyDestination(context.Background(), cfg, dest); err == nil || !errors.Is(err, exitcode.ErrPrecondition) {
		t.Fatalf("expected precondition error, got %v", err)
	}
}
