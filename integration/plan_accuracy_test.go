//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPlanAccuracy(t *testing.T) {
	requireRootAndCommands(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "lvmsync")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lvmsync: %v\n%s", err, out)
	}

	size := 4 * 1024 * 1024 // 4MiB
	src := filepath.Join(tmp, "src.bin")
	if err := os.WriteFile(src, make([]byte, size), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dst := filepath.Join(tmp, "dst.bin")

	planCmd := exec.Command(bin, "run", "--plan", src, dst)
	planOut, err := planCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, planOut)
	}
	var po struct {
		EstimatedBytes int64 `json:"estimated_bytes"`
	}
	if err := json.Unmarshal(planOut, &po); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, planOut)
	}

	runCmd := exec.Command(bin, "run", "--output=json", src, dst)
	runOut, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, runOut)
	}
	var transferred int64
	scanner := bufio.NewScanner(bytes.NewReader(runOut))
	for scanner.Scan() {
		var evt struct {
			BytesTransferred int64 `json:"bytes_transferred"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &evt); err == nil && evt.BytesTransferred > 0 {
			transferred = evt.BytesTransferred
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan output: %v", err)
	}
	if transferred == 0 {
		t.Fatalf("bytes_transferred not found\n%s", runOut)
	}

	diff := math.Abs(float64(po.EstimatedBytes-transferred)) / float64(transferred)
	const tolerance = 0.05
	if diff > tolerance {
		t.Fatalf("plan estimate %d vs actual %d exceeds tolerance %.2f", po.EstimatedBytes, transferred, tolerance)
	}
}
