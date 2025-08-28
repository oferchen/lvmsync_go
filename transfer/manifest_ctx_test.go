//go:build unix

package transfer

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	manifestpkg "github.com/oferchen/lvmsync_go/manifest"
)

func writeHeader(t *testing.T, path string) {
	var hdr manifestpkg.Header
	hdr.Version = manifestpkg.Version
	copy(hdr.DeviceID[:], []byte("dev"))
	hdr.MAC = manifestHeaderMAC(&hdr)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create header: %v", err)
	}
	if err := binary.Write(f, binary.LittleEndian, &hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	f.Close()
}

func TestReadManifestHeaderNilContext(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "man")
	writeHeader(t, p)
	//lint:ignore SA1012 testing nil context handling
	if _, err := readManifestHeader(nil, p, 0); err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("expected nil context error, got %v", err)
	}
}

func TestReadManifestHeaderCanceled(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "man")
	writeHeader(t, p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readManifestHeader(ctx, p, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestReadManifestHeaderTimeout(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		w, _ := os.OpenFile(fifo, os.O_WRONLY, 0600)
		if w != nil {
			w.Close()
		}
		close(done)
	}()
	if _, err := readManifestHeader(ctx, fifo, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	<-done
}
