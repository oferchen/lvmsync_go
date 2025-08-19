//go:build linux

package transfer

import (
	"os"
	"reflect"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/sys/unix"

	"lvmsync_go/internal/config"
)

func TestParseCPUList(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"0-2,4,6-7", []int{0, 1, 2, 4, 6, 7}},
		{"1", []int{1}},
		{"", nil},
	}
	for _, c := range cases {
		got := parseCPUList(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("len mismatch for %q: %v vs %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("mismatch for %q at %d: %d vs %d", c.in, i, got[i], c.want[i])
			}
		}
	}
}

type fakeNumaOps struct {
	files map[string][]byte
	mask  unix.CPUSet
}

func (f *fakeNumaOps) ReadFile(p string) ([]byte, error) {
	if f.files == nil {
		return nil, os.ErrNotExist
	}
	b, ok := f.files[p]
	if !ok {
		return nil, os.ErrNotExist
	}
	return b, nil
}

func (f *fakeNumaOps) SchedSetaffinity(_ int, m *unix.CPUSet) error {
	f.mask = *m
	return nil
}

func cpusFromMask(m *unix.CPUSet) []int {
	var cpus []int
	for i := 0; i < 64; i++ {
		if m.IsSet(i) {
			cpus = append(cpus, i)
		}
	}
	return cpus
}

func TestPinCurrentThreadToNodeMock(t *testing.T) {
	ops := &fakeNumaOps{files: map[string][]byte{
		"/sys/devices/system/node/node2/cpulist": []byte("4-5"),
	}}
	if err := pinCurrentThreadToNodeWithOps(2, ops); err != nil {
		t.Fatalf("pin: %v", err)
	}
	got := cpusFromMask(&ops.mask)
	want := []int{4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cpuset %v != %v", got, want)
	}
}

func TestPinCurrentThreadToDeviceMock(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ops := &fakeNumaOps{files: map[string][]byte{
		"/sys/dev/block/0:0/numa_node":           []byte("1"),
		"/sys/devices/system/node/node1/cpulist": []byte("0-1"),
	}}
	if err := pinCurrentThreadToDeviceWithOps(tmp, ops); err != nil {
		t.Fatalf("pin: %v", err)
	}
	got := cpusFromMask(&ops.mask)
	want := []int{0, 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cpuset %v != %v", got, want)
	}
}

func TestPinCurrentThreadToNodeMissing(t *testing.T) {
	ops := &fakeNumaOps{}
	if err := pinCurrentThreadToNodeWithOps(3, ops); err == nil {
		t.Fatalf("expected error")
	}
}

func TestPinCurrentThreadToNodeReal(t *testing.T) {
	if err := pinCurrentThreadToNode(0); err != nil {
		if os.IsNotExist(err) {
			t.Skipf("numa unsupported: %v", err)
		}
		t.Logf("pin failed: %v", err)
	}
}

func TestPinWorkerToDeviceMissingNUMAInfo(t *testing.T) {
	cfg := &config.Config{NumaPin: true, NumaNode: -1}
	tmp, err := os.CreateTemp(t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	unlock := pinWorkerToDevice(cfg, tmp, logger)
	unlock()
	if logs.FilterMessage("numa pin failed").Len() != 1 {
		t.Fatalf("expected numa pin warning, got %d", logs.FilterMessage("numa pin failed").Len())
	}
}
