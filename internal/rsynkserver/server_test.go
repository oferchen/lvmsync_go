package rsynkserver

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lvmsync_go/internal/rsynkwire"

	"github.com/gokrazy/rsync/rsyncclient"
)

// helper to run client and server over an in-memory connection
func run(ctx context.Context, t *testing.T, client *rsynkwire.Client, server *Server, src, dest string) (*rsyncclient.Result, error) {
	c1, c2 := net.Pipe()
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Handle(ctx, c1, client.ServerCommandOptions(dest+string(os.PathSeparator)))
	}()
	res, err := client.Run(ctx, c2, []string{src})
	c2.Close()
	if err != nil {
		<-errCh
		return nil, err
	}
	if serr := <-errCh; serr != nil {
		return nil, serr
	}
	return res, nil
}

func TestNegotiation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	destDir := filepath.Join(tmp, "dest")
	data := []byte("hello world")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}

	client, err := rsynkwire.NewClient([]string{"-av"}, rsyncclient.WithSender())
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}

	res, err := run(ctx, t, client, srv, src, destDir)
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, filepath.Base(src)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("dest mismatch: got %q", got)
	}
	if res.Stats.Read == 0 || res.Stats.Written == 0 {
		t.Fatalf("unexpected zero stats: %+v", res.Stats)
	}
}

func TestDeltaTransfer(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	destDir := filepath.Join(tmp, "dest")
	srcData := bytes.Repeat([]byte("a"), 32*1024)
	if err := os.WriteFile(src, srcData, 0o644); err != nil {
		t.Fatal(err)
	}
	destData := append([]byte{}, srcData...)
	destData[0] = 'b'
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	destFile := filepath.Join(destDir, filepath.Base(src))
	if err := os.WriteFile(destFile, destData, 0o644); err != nil {
		t.Fatal(err)
	}
	// Ensure modification time differs so rsync considers the file outdated.
	if err := os.Chtimes(destFile, time.Unix(0, 0), time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}

	client, err := rsynkwire.NewClient([]string{"-av"}, rsyncclient.WithSender())
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}

	res, err := run(ctx, t, client, srv, src, destDir)
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}

	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, srcData) {
		t.Fatalf("dest mismatch after delta transfer")
	}
	if res.Stats.Written >= res.Stats.Size {
		t.Fatalf("expected delta transfer, wrote %d bytes for size %d", res.Stats.Written, res.Stats.Size)
	}
}
