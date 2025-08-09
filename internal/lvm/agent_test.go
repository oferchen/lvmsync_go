package lvm

import (
	"context"
	"errors"
	"testing"
)

type mockLVM struct {
	lockErr         error
	unlockErr       error
	md              VolumeMetadata
	getMetadataErr  error
	sendMetadataErr error
	startErr        error
	finalizeErr     error
	status          string
	statusErr       error
}

func (m *mockLVM) Lock(_ context.Context, _, _ string) error {
	return m.lockErr
}

func (m *mockLVM) Unlock(_ context.Context, _, _ string) error {
	return m.unlockErr
}

func (m *mockLVM) GetMetadata(_ context.Context, _ string) (VolumeMetadata, error) {
	return m.md, m.getMetadataErr
}

func (m *mockLVM) SendMetadata(_ context.Context, md VolumeMetadata) error {
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

func TestSudoAgentLock(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{}
	a := NewSudoAgent("", mock)
	if err := a.Lock(ctx, "vol", "req"); err != nil {
		t.Fatalf("lock failed: %v", err)
	}
	mock.lockErr = errors.New("boom")
	if err := a.Lock(ctx, "vol", "req"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSudoAgentUnlock(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{}
	a := NewSudoAgent("", mock)
	if err := a.Unlock(ctx, "vol", "req"); err != nil {
		t.Fatalf("unlock failed: %v", err)
	}
	mock.unlockErr = errors.New("boom")
	if err := a.Unlock(ctx, "vol", "req"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSudoAgentGetMetadata(t *testing.T) {
	ctx := context.Background()
	expected := VolumeMetadata{VolumeName: "vol", SizeBytes: 1, ChunkSize: 2}
	mock := &mockLVM{md: expected}
	a := NewSudoAgent("", mock)
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

func TestSudoAgentSendMetadata(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{}
	a := NewSudoAgent("", mock)
	md := VolumeMetadata{VolumeName: "vol"}
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

func TestSudoAgentStartTransferSession(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{}
	a := NewSudoAgent("", mock)
	if err := a.StartTransferSession(ctx, "vol", "req"); err != nil {
		t.Fatalf("start transfer failed: %v", err)
	}
	mock.startErr = errors.New("boom")
	if err := a.StartTransferSession(ctx, "vol", "req"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSudoAgentFinalizeSync(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{}
	a := NewSudoAgent("", mock)
	if err := a.FinalizeSync(ctx, "vol", "req"); err != nil {
		t.Fatalf("finalize sync failed: %v", err)
	}
	mock.finalizeErr = errors.New("boom")
	if err := a.FinalizeSync(ctx, "vol", "req"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSudoAgentGetStatus(t *testing.T) {
	ctx := context.Background()
	mock := &mockLVM{status: "ok"}
	a := NewSudoAgent("", mock)
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
