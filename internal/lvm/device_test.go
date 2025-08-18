package lvm

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

type fakeFile struct {
	synced bool
	err    error
}

func (f *fakeFile) Sync() error {
	f.synced = true
	return f.err
}

type preMock struct {
	Agent
	exists, auto, discard, mounted bool
	lockErr                        error
}

func (m *preMock) VolumeExists(context.Context, string) (bool, error)      { return m.exists, nil }
func (m *preMock) AutoExtendEnabled(context.Context, string) (bool, error) { return m.auto, nil }
func (m *preMock) DiscardEnabled(context.Context, string) (bool, error)    { return m.discard, nil }
func (m *preMock) IsMounted(context.Context, string) (bool, error)         { return m.mounted, nil }
func (m *preMock) Lock(context.Context, string, string) error              { return m.lockErr }
func (m *preMock) Unlock(context.Context, string, string) error            { return nil }

func TestCheckerPreOpen(t *testing.T) {
	ctx := context.Background()
	mock := &preMock{exists: true, auto: true, discard: true}
	c := Checker{Agent: mock, Requester: "req"}
	path, err := c.PreOpen(ctx, "vg", "lv")
	if err != nil {
		t.Fatalf("preopen: %v", err)
	}
	if path != "/dev/vg/lv" {
		t.Fatalf("path %s", path)
	}
	root := t.TempDir()
	c.DevRoot = root
	path, err = c.PreOpen(ctx, "vg", "lv")
	if err != nil {
		t.Fatalf("preopen with root: %v", err)
	}
	want := filepath.Join(root, "vg", "lv")
	if path != want {
		t.Fatalf("path %s want %s", path, want)
	}
	mock.exists = false
	if _, err := c.PreOpen(ctx, "vg", "lv"); err == nil {
		t.Fatalf("expected error for missing volume")
	}
}

func TestCheckerPreOpenInvalidNames(t *testing.T) {
	ctx := context.Background()
	mock := &preMock{exists: true, auto: true, discard: true}
	c := Checker{Agent: mock, Requester: "req"}
	cases := []struct{ vg, lv string }{
		{"vg/foo", "lv"},
		{"vg", "lv/bar"},
		{"..", "lv"},
		{"vg", ".."},
		{"", "lv"},
		{"vg", ""},
	}
	for _, tc := range cases {
		if _, err := c.PreOpen(ctx, tc.vg, tc.lv); err == nil {
			t.Fatalf("expected error for vg %q lv %q", tc.vg, tc.lv)
		}
	}
}

func TestCheckerPostCommit(t *testing.T) {
	ctx := context.Background()
	f := &fakeFile{}
	mock := &preMock{exists: true, auto: true, discard: true}
	c := Checker{Agent: mock, Requester: "req"}
	if err := c.PostCommit(ctx, "vg", "lv", f); err != nil {
		t.Fatalf("postcommit: %v", err)
	}
	if !f.synced {
		t.Fatalf("expected sync")
	}
	f.err = errors.New("boom")
	if err := c.PostCommit(ctx, "vg", "lv", f); err == nil {
		t.Fatalf("expected sync error")
	}
}
