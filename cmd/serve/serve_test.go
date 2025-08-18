package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestServeCommandDeprecated(t *testing.T) {
	var buf bytes.Buffer
	code := run(nil, &buf)
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if !strings.Contains(buf.String(), "lvmsyncd") {
		t.Fatalf("expected message to mention lvmsyncd, got %q", buf.String())
	}
}
