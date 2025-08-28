package lvm

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"go.uber.org/zap"

	privilege "github.com/oferchen/lvmsync_go/internal/privilege"
	lvmlib "github.com/oferchen/lvmsync_go/lvm"
)

type fakeEsc struct{ err error }

func (f fakeEsc) Ensure(context.Context) error { return f.err }
func (fakeEsc) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

type mockLVM struct {
	lockErr         error
	unlockErr       error
	md              lvmlib.VolumeMetadata
	getMetadataErr  error
	sendMetadataErr error
	startErr        error
	finalizeErr     error
	status          string
	statusErr       error
	exists          bool
	existsErr       error
	autoExtend      bool
	autoErr         error
	discard         bool
	discardErr      error
	mounted         bool
	mountedErr      error
}

func (m *mockLVM) Lock(_ context.Context, _, _ string) error {
	return m.lockErr
}

func (m *mockLVM) Unlock(_ context.Context, _, _ string) error {
	return m.unlockErr
}

func (m *mockLVM) GetMetadata(_ context.Context, _ string) (lvmlib.VolumeMetadata, error) {
	return m.md, m.getMetadataErr
}

func (m *mockLVM) SendMetadata(_ context.Context, md lvmlib.VolumeMetadata) error {
	m.md = md
	return m.sendMetadataErr
}

func (m *mockLVM) StartTransferSession(_ context.Context, _, _ string) error {
	return m.startErr
}

func (m *mockLVM) FinalizeSync(_ context.Context, _, _ string) error {
	return m.finalizeErr
}

func (m *mockLVM) GetStatus(_ context.Context, _, _ string) (string, error) {
	return m.status, m.statusErr
}

func (m *mockLVM) VolumeExists(_ context.Context, _ string) (bool, error) {
	return m.exists, m.existsErr
}

func (m *mockLVM) AutoExtendEnabled(_ context.Context, _ string) (bool, error) {
	return m.autoExtend, m.autoErr
}

func (m *mockLVM) DiscardEnabled(_ context.Context, _ string) (bool, error) {
	return m.discard, m.discardErr
}

func (m *mockLVM) IsMounted(_ context.Context, _ string) (bool, error) {
	return m.mounted, m.mountedErr
}

var _ lvmlib.API = (*mockLVM)(nil)

func TestAgentLock(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{}
	a := NewAgent(mock, fakeEsc{}, zap.NewNop())
	if err := a.Lock(ctx, "vol", "req"); err != nil {
		t.Fatalf("lock failed: %v", err)
	}
	mock.lockErr = errors.New("boom")
	if err := a.Lock(ctx, "vol", "req"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAgentUnlock(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{}
	a := NewAgent(mock, fakeEsc{}, zap.NewNop())
	if err := a.Unlock(ctx, "vol", "req"); err != nil {
		t.Fatalf("unlock failed: %v", err)
	}
	mock.unlockErr = errors.New("boom")
	if err := a.Unlock(ctx, "vol", "req"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAgentGetMetadata(t *testing.T) {
	ctx := context.Background()
	expected := lvmlib.VolumeMetadata{VolumeName: "vol", SizeBytes: 1, ChunkSize: 2}
	mock := &mockLVM{md: expected}
	a := NewAgent(mock, fakeEsc{}, zap.NewNop())
	md, err := a.GetMetadata(ctx, "vol")
	if err != nil {
		t.Fatalf("get metadata failed: %v", err)
	}
	if md != expected {
		t.Fatalf("unexpected metadata: %+v", md)
	}
	mock.getMetadataErr = errors.New("boom")
	if _, err := a.GetMetadata(ctx, "vol"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAgentSendMetadata(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{}
	a := NewAgent(mock, fakeEsc{}, zap.NewNop())
	md := lvmlib.VolumeMetadata{VolumeName: "vol"}
	if err := a.SendMetadata(ctx, md); err != nil {
		t.Fatalf("send metadata failed: %v", err)
	}
	if mock.md != md {
		t.Fatalf("metadata not forwarded: %+v", mock.md)
	}
	mock.sendMetadataErr = errors.New("boom")
	if err := a.SendMetadata(ctx, md); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAgentStartTransferSession(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{}
	a := NewAgent(mock, fakeEsc{}, zap.NewNop())
	if err := a.StartTransferSession(ctx, "vol", "req"); err != nil {
		t.Fatalf("start transfer failed: %v", err)
	}
	mock.startErr = errors.New("boom")
	if err := a.StartTransferSession(ctx, "vol", "req"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAgentFinalizeSync(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{}
	a := NewAgent(mock, fakeEsc{}, zap.NewNop())
	if err := a.FinalizeSync(ctx, "vol", "req"); err != nil {
		t.Fatalf("finalize sync failed: %v", err)
	}
	mock.finalizeErr = errors.New("boom")
	if err := a.FinalizeSync(ctx, "vol", "req"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAgentGetStatus(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{status: "ok"}
	a := NewAgent(mock, fakeEsc{}, zap.NewNop())
	status, err := a.GetStatus(ctx, "vol", "req")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	if status != "ok" {
		t.Fatalf("unexpected status: %s", status)
	}
	mock.statusErr = errors.New("boom")
	if _, err := a.GetStatus(ctx, "vol", "req"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAgentVolumeExists(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{exists: true}
	a := NewAgent(mock, fakeEsc{}, zap.NewNop())
	ok, err := a.VolumeExists(ctx, "vol")
	if err != nil || !ok {
		t.Fatalf("volume exists check failed: %v %v", ok, err)
	}
	mock.existsErr = errors.New("boom")
	if _, err := a.VolumeExists(ctx, "vol"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAgentAutoExtendEnabled(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{autoExtend: true}
	a := NewAgent(mock, fakeEsc{}, zap.NewNop())
	ok, err := a.AutoExtendEnabled(ctx, "vol")
	if err != nil || !ok {
		t.Fatalf("auto extend check failed: %v %v", ok, err)
	}
	mock.autoErr = errors.New("boom")
	if _, err := a.AutoExtendEnabled(ctx, "vol"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAgentDiscardEnabled(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{discard: true}
	a := NewAgent(mock, fakeEsc{}, zap.NewNop())
	ok, err := a.DiscardEnabled(ctx, "vol")
	if err != nil || !ok {
		t.Fatalf("discard check failed: %v %v", ok, err)
	}
	mock.discardErr = errors.New("boom")
	if _, err := a.DiscardEnabled(ctx, "vol"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAgentIsMounted(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{mounted: true}
	a := NewAgent(mock, fakeEsc{}, zap.NewNop())
	ok, err := a.IsMounted(ctx, "vol")
	if err != nil || !ok {
		t.Fatalf("mount check failed: %v %v", ok, err)
	}
	mock.mountedErr = errors.New("boom")
	if _, err := a.IsMounted(ctx, "vol"); err == nil {
		t.Fatalf("expected error")
	}
}

// TestNewSudoAgent requires root or appropriate capabilities.
func TestNewSudoAgent(t *testing.T) {
	ctx := context.Background()
	esc, err := privilege.New(context.Background(), zap.NewNop())
	if err != nil {
		t.Fatalf("privilege.New: %v", err)
	}
	if err := esc.Ensure(ctx); err != nil {
		t.Skipf("requires root: %v", err)
	}
	mock := &mockLVM{exists: true}
	a := NewSudoAgent("", mock, nil, zap.NewNop())
	ok, err := a.VolumeExists(ctx, "vol")
	if err != nil || !ok {
		t.Fatalf("VolumeExists failed: %v %v", ok, err)
	}
}

// TestNewSudoAgentNilAPI requires root or appropriate capabilities.
func TestNewSudoAgentNilAPI(t *testing.T) {
	ctx := context.Background()
	esc, err := privilege.New(context.Background(), zap.NewNop())
	if err != nil {
		t.Fatalf("privilege.New: %v", err)
	}
	if err := esc.Ensure(ctx); err != nil {
		t.Skipf("requires root: %v", err)
	}
	a := NewSudoAgent("", nil, nil, zap.NewNop())
	if err := a.Lock(ctx, "vol", "req"); err == nil {
		t.Fatalf("expected error")
	}
}
