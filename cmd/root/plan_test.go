package root

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)

type planResult struct {
	TransportOrder []string                  `json:"transport_order"`
	EstimatedBytes int64                     `json:"estimated_bytes"`
	Compression    map[string]map[string]any `json:"compression"`
}

func TestRunPlanOutputsJSON(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	cfg := &config.Config{
		Plan:      true,
		Transport: "ssh,tcp+tls",
		DedupMode: "cdc",
		CDCMin:    64 * 1024,
		CDCAvg:    256 * 1024,
		CDCMax:    512 * 1024,
		LZ4Level:  "fast",
		ZstdLevel: 1,
		Compress:  "auto",
	}
	logger := zap.NewNop()
	r := NewRunnerWithDeps(nil)
	var buf bytes.Buffer
	oldStdout := os.Stdout
	rpr, wp, _ := os.Pipe()
	os.Stdout = wp
	done := make(chan struct{})
	go func() {
		io.Copy(&buf, rpr)
		close(done)
	}()
	err := r.Run(cfg, []string{src, "dst"}, logger)
	wp.Close()
	<-done
	os.Stdout = oldStdout
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var res planResult
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.EstimatedBytes != 4 {
		t.Fatalf("EstimatedBytes=%d want %d", res.EstimatedBytes, 4)
	}
	if len(res.TransportOrder) != 2 || res.TransportOrder[0] != "ssh" {
		t.Fatalf("unexpected transport order %v", res.TransportOrder)
	}
	if len(res.Compression) == 0 {
		t.Fatalf("compression plan empty")
	}
}
