package blockio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/oferchen/lvmsync_go/internal/lvm"
)

type mockAgent struct {
	volumeExists bool
	autoExtend   bool
	discard      bool
	mounted      bool
	lockCalled   bool
	unlockCalled bool
	unlockErr    error
}

func (m *mockAgent) Lock(ctx context.Context, volume, requester string) error {
	m.lockCalled = true
	return nil
}

func (m *mockAgent) Unlock(ctx context.Context, volume, requester string) error {
	m.unlockCalled = true
	return m.unlockErr
}

func (m *mockAgent) GetMetadata(ctx context.Context, volume string) (lvm.VolumeMetadata, error) {
	return lvm.VolumeMetadata{}, nil
}

func (m *mockAgent) SendMetadata(ctx context.Context, md lvm.VolumeMetadata) error { return nil }

func (m *mockAgent) StartTransferSession(ctx context.Context, volume, requester string) error {
	return nil
}

func (m *mockAgent) FinalizeSync(ctx context.Context, volume, requester string) error { return nil }

func (m *mockAgent) GetStatus(ctx context.Context, volume, requester string) (string, error) {
	return "", nil
}

func (m *mockAgent) VolumeExists(ctx context.Context, volume string) (bool, error) {
	return m.volumeExists, nil
}

func (m *mockAgent) AutoExtendEnabled(ctx context.Context, volume string) (bool, error) {
	return m.autoExtend, nil
}

func (m *mockAgent) DiscardEnabled(ctx context.Context, volume string) (bool, error) {
	return m.discard, nil
}

func (m *mockAgent) IsMounted(ctx context.Context, volume string) (bool, error) {
	return m.mounted, nil
}

func TestDeviceWriterOpenSuccess(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "testvg1", "testlv1")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ftmp, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ftmp.Close()

	agent := &mockAgent{volumeExists: true, autoExtend: true, discard: true}
	dw := DeviceWriter{Checker: lvm.Checker{Agent: agent, Requester: "req", DevRoot: root}}

	f, closeFn, err := dw.Open(ctx, "testvg1", "testlv1", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if f == nil || closeFn == nil {
		t.Fatalf("expected file and close function")
	}
	if f.Name() != path {
		t.Fatalf("file path %q want %q", f.Name(), path)
	}
	if !agent.lockCalled {
		t.Fatalf("expected lock called")
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !agent.unlockCalled {
		t.Fatalf("expected unlock called")
	}
}

func TestDeviceWriterOpenPreOpenFailure(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	agent := &mockAgent{volumeExists: false}
	dw := DeviceWriter{Checker: lvm.Checker{Agent: agent, Requester: "req", DevRoot: root}}
	f, closeFn, err := dw.Open(ctx, "vg", "lv", false)
	if err == nil {
		t.Fatalf("expected error")
	}
	if f != nil || closeFn != nil {
		t.Fatalf("expected nil file and closeFn")
	}
	if agent.lockCalled || agent.unlockCalled {
		t.Fatalf("lock/unlock should not be called")
	}
}

func TestDeviceWriterOpenPostCommitFailure(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "testvg2", "testlv2")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ftmp, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ftmp.Close()

	agent := &mockAgent{volumeExists: true, autoExtend: true, discard: true, unlockErr: errors.New("unlock failed")}
	dw := DeviceWriter{Checker: lvm.Checker{Agent: agent, Requester: "req", DevRoot: root}}

	f, closeFn, err := dw.Open(ctx, "testvg2", "testlv2", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if f == nil || closeFn == nil {
		t.Fatalf("expected file and close function")
	}
	if f.Name() != path {
		t.Fatalf("file path %q want %q", f.Name(), path)
	}
	if err := closeFn(); err == nil || err.Error() != "unlock failed" {
		t.Fatalf("unexpected close error: %v", err)
	}
	if !agent.unlockCalled {
		t.Fatalf("expected unlock attempt on error")
	}
}
