//go:build integration

package transfer

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
	"lvmsync_go/transport"
)

// compareFiles verifies two paths have identical contents.
func compareFiles(t *testing.T, a, b string) {
	t.Helper()
	da, err := os.ReadFile(a)
	if err != nil {
		t.Fatalf("read %s: %v", a, err)
	}
	db, err := os.ReadFile(b)
	if err != nil {
		t.Fatalf("read %s: %v", b, err)
	}
	if !bytes.Equal(da, db) {
		t.Fatalf("file mismatch")
	}
}

// TestReconnectMidTransfer drops the connection and verifies the transfer resumes.
func TestReconnectMidTransfer(t *testing.T) {
	blockSize := 4096
	snapshot := "vg-lv"
	_, src := createVolumeFiles(t, snapshot, int64(blockSize), []int{0, 1, 2, 3, 4, 5, 6, 7})
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.WriteFile(dest, make([]byte, blockSize*8), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	senderState := filepath.Join(t.TempDir(), "sender.state")
	applyState := filepath.Join(t.TempDir(), "apply.state")

	applyCfg := &config.Config{BlockSize: blockSize, Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: applyState}
	tr := NewTransfer(zap.NewNop(), nil, nil)

	trNet, err := transport.Get("tcp+tls", transport.Config{Logger: zap.NewNop(), AllowInsecure: true})
	if err != nil {
		t.Fatalf("get transport: %v", err)
	}
	ctx := context.Background()
	ln, err := trNet.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	serverDone := make(chan error, 1)
	go func() {
		first := true
		for {
			conn, err := ln.Accept()
			if err != nil {
				serverDone <- err
				return
			}
			if first {
				first = false
				go func(c net.Conn) {
					time.Sleep(200 * time.Millisecond)
					c.Close()
				}(conn)
			}
			tt := NewTransfer(zap.NewNop(), nil, nil)
			err = tt.ProcessDumpData(context.Background(), applyCfg, conn, dest)
			conn.Close()
			if err == nil {
				serverDone <- nil
				return
			}
		}
	}()

	cfg := &config.Config{
		BlockSize:         blockSize,
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		ResumeState:       senderState,
		ResumeToken:       "tok",
		CheckpointBytes:   1,
		SpeedLimit:        1024,
		MaxRetries:        3,
		RetryDelay:        200 * time.Millisecond,
	}
	if err := tr.DumpChangesWithReconnect(ctx, cfg, trNet, ln.Addr().String(), snapshot, src); err != nil {
		t.Fatalf("DumpChangesWithReconnect: %v", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server: %v", err)
	}
	compareFiles(t, src, dest)
}
