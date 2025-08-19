//go:build integration && linux

package transfer

import (
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func cpusFromMask(m *unix.CPUSet) []int {
	var cpus []int
	for i := 0; i < 64; i++ {
		if m.IsSet(i) {
			cpus = append(cpus, i)
		}
	}
	return cpus
}

func TestPinCurrentThreadToNodeIntegration(t *testing.T) {
	if _, err := os.Stat("/sys/devices/system/node/node1/cpulist"); err != nil {
		t.Skip("requires system with multiple NUMA nodes")
	}
	var orig unix.CPUSet
	if err := unix.SchedGetaffinity(0, &orig); err != nil {
		t.Skipf("get affinity: %v", err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer unix.SchedSetaffinity(0, &orig)

	if err := pinCurrentThreadToNode(1); err != nil {
		t.Fatalf("pin: %v", err)
	}
	var mask unix.CPUSet
	if err := unix.SchedGetaffinity(0, &mask); err != nil {
		t.Fatalf("get affinity: %v", err)
	}
	got := cpusFromMask(&mask)
	b, err := os.ReadFile("/sys/devices/system/node/node1/cpulist")
	if err != nil {
		t.Fatalf("read cpulist: %v", err)
	}
	want := parseCPUList(strings.TrimSpace(string(b)))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cpuset %v != %v", got, want)
	}
}
