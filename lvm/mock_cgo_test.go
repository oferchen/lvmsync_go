package lvm

import (
    "fmt"

    "lvmsync_go/lvm/cgo"
)

// mockCGO implements the cgo.LVM interface for tests and records calls.
type mockCGO struct {
    calls  []string
    usage  float64
    vgFree map[string]uint64
    vgs    []cgo.VolumeGroup
}

func newMockCGO() *mockCGO { return &mockCGO{vgFree: make(map[string]uint64)} }

func (m *mockCGO) CreateSnapshot(lvPath, snapName string, sizeBytes uint64) error {
    m.calls = append(m.calls, fmt.Sprintf("create:%s:%s:%d", lvPath, snapName, sizeBytes))
    return nil
}

func (m *mockCGO) RemoveLV(lvPath string) error {
    m.calls = append(m.calls, fmt.Sprintf("remove:%s", lvPath))
    return nil
}

func (m *mockCGO) SnapshotUsage(string) (float64, error) { return m.usage, nil }

func (m *mockCGO) VGFree(vgName string) (uint64, error) {
    if free, ok := m.vgFree[vgName]; ok {
        return free, nil
    }
    return 0, fmt.Errorf("unknown vg")
}

func (m *mockCGO) ListVGs() ([]cgo.VolumeGroup, error) { return m.vgs, nil }

