//go:build linux

package transfer

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/config"
)

// pinCurrentThreadToDevice pins the current OS thread to CPUs local to the
// NUMA node of the provided device file.
func pinCurrentThreadToDevice(f *os.File) error {
	st, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat device: %w", err)
	}
	sys, ok := st.Sys().(*unix.Stat_t)
	if !ok {
		return fmt.Errorf("unexpected stat type")
	}
	major := unix.Major(uint64(sys.Rdev))
	minor := unix.Minor(uint64(sys.Rdev))
	nodePath := fmt.Sprintf("/sys/dev/block/%d:%d/numa_node", major, minor)
	b, err := os.ReadFile(nodePath)
	if err != nil {
		return err
	}
	node, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || node < 0 {
		return fmt.Errorf("invalid numa node")
	}
	cpuListPath := fmt.Sprintf("/sys/devices/system/node/node%d/cpulist", node)
	cpulist, err := os.ReadFile(cpuListPath)
	if err != nil {
		return err
	}
	cpus := parseCPUList(strings.TrimSpace(string(cpulist)))
	if len(cpus) == 0 {
		return fmt.Errorf("no cpus for node %d", node)
	}
	var mask unix.CPUSet
	for _, cpu := range cpus {
		mask.Set(cpu)
	}
	if err := unix.SchedSetaffinity(0, &mask); err != nil {
		return fmt.Errorf("set affinity: %w", err)
	}
	return nil
}

func parseCPUList(list string) []int {
	var cpus []int
	for _, part := range strings.Split(list, ",") {
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, err1 := strconv.Atoi(bounds[0])
			end, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil {
				continue
			}
			for i := start; i <= end; i++ {
				cpus = append(cpus, i)
			}
		} else {
			v, err := strconv.Atoi(part)
			if err != nil {
				continue
			}
			cpus = append(cpus, v)
		}
	}
	return cpus
}

// pinWorkerToDevice pins the current goroutine to the NUMA node associated
// with the source device when cfg.NumaPin is true. The returned function must
// be deferred to release the thread lock.
func pinWorkerToDevice(cfg *config.Config, src *os.File) func() {
	if cfg == nil || !cfg.NumaPin {
		return func() {}
	}
	runtime.LockOSThread()
	if err := pinCurrentThreadToDevice(src); err != nil {
		if Logger != nil {
			Logger.Warn("numa pin failed", zap.Error(err))
		}
	}
	return runtime.UnlockOSThread
}
