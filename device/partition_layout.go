package device

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var sfdiskPath string

func init() {
	sfdiskPath, _ = exec.LookPath("sfdisk")
}

func readPartitionLayout(ctx context.Context, path string, runner *Runner) ([]partition, error) {
	if sfdiskPath == "" {
		return nil, fmt.Errorf("sfdisk not found")
	}
	if runner == nil {
		runner = NewRunner()
	}
	out, err := runner.command.CommandContext(ctx, sfdiskPath, "-d", path).Output()
	if err != nil {
		return nil, err
	}
	return parseSfdiskDump(string(out))
}

var sfdiskLine = regexp.MustCompile(`^/[^ ]+ : start=\s*(\d+), size=\s*(\d+), type=(\S+)`)

func parseSfdiskDump(out string) ([]partition, error) {
	var parts []partition
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := sfdiskLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		start, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			return nil, err
		}
		size, err := strconv.ParseUint(m[2], 10, 64)
		if err != nil {
			return nil, err
		}
		end := start + size - 1
		parts = append(parts, partition{Start: start, End: end, Type: m[3]})
	}
	return parts, nil
}

type partitionDiff struct {
	Index int        `json:"index"`
	A     *partition `json:"a,omitempty"`
	B     *partition `json:"b,omitempty"`
}

func diffPartitionLayouts(a, b []partition) []partitionDiff {
	max := len(a)
	if len(b) > max {
		max = len(b)
	}
	var diffs []partitionDiff
	for i := 0; i < max; i++ {
		var pa, pb *partition
		if i < len(a) {
			pa = &a[i]
		}
		if i < len(b) {
			pb = &b[i]
		}
		if pa == nil || pb == nil || pa.Start != pb.Start || pa.End != pb.End || pa.Type != pb.Type {
			diffs = append(diffs, partitionDiff{Index: i, A: pa, B: pb})
		}
	}
	return diffs
}
