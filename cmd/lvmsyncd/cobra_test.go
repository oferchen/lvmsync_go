package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

func TestFlagParsing(t *testing.T) {
	logger := zap.NewNop()
	var got Options
	r := NewRunnerWithDeps(func(ctx context.Context, opts Options, _ *zap.Logger, _ func(string, transport.Config) (transport.Interface, error)) error {
		got = opts
		return nil
	}, nil)
	args := []string{
		"--listen", "tcp://:8080",
		"--listen", "unix:///tmp/sock",
		"--module", "mod1",
		"--module", "mod2",
		"--sudo-helper", "/bin/helper",
		"--once",
	}
	if err := r.Execute(args, logger); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := Options{
		Listen:     []string{"tcp://:8080", "unix:///tmp/sock"},
		Modules:    []string{"mod1", "mod2"},
		Once:       true,
		SudoHelper: "/bin/helper",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

type bindErrViper struct {
	*viper.Viper
}

func (b *bindErrViper) BindPFlags(_ *pflag.FlagSet) error { return errors.New("bind fail") }
func (b *bindErrViper) Underlying() *viper.Viper          { return b.Viper }

func TestNewCmdBindError(t *testing.T) {
	r := NewRunner()
	if _, err := r.NewCmd(zap.NewNop(), &bindErrViper{Viper: viper.New()}); err == nil || err.Error() != "bind fail" {
		t.Fatalf("expected bind fail, got %v", err)
	}
}

func TestLoadConfigUnknownKey(t *testing.T) {
	v := viper.New()
	if err := bindFlagSets(&cobra.Command{}, v); err != nil {
		t.Fatalf("bindFlagSets: %v", err)
	}
	cfgPath := writeTempConfig(t, "stray: 1\n")
	v.Set("config", cfgPath)
	_, warns, err := loadConfig(v)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(warns) != 1 || warns[0] != `unknown configuration key "stray"` {
		t.Fatalf("unexpected warnings %v", warns)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path

type trackingListener struct {
	net.Listener
	mu     sync.Mutex
	closed bool
}

func (l *trackingListener) Close() error {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	return l.Listener.Close()
}

func (l *trackingListener) Closed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

type fakeTransport struct {
	listen func(context.Context, string) (net.Listener, error)
}

func (f fakeTransport) Name() string                                   { return "fake" }
func (f fakeTransport) Dial(context.Context, string) (net.Conn, error) { return nil, nil }
func (f fakeTransport) Listen(ctx context.Context, addr string) (net.Listener, error) {
	if f.listen != nil {
		return f.listen(ctx, addr)
	}
	return nil, nil
}
func (f fakeTransport) Negotiate(ctx context.Context, conn net.Conn, role transport.Role, hs common.Handshake) (common.Handshake, error) {
	return hs, nil
}

func TestStartContextCancelStopsListeners(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var tl *trackingListener
	get := func(string, transport.Config) (transport.Interface, error) {
		ft := fakeTransport{listen: func(ctx context.Context, addr string) (net.Listener, error) {
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return nil, err
			}
			tl = &trackingListener{Listener: ln}
			go func() {
				<-ctx.Done()
				tl.Close()
			}()
			return tl, nil
		}}
		return ft, nil
	}

	opts := Options{Listen: []string{"grpc://127.0.0.1:0", "fake://127.0.0.1:0"}}
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	err := start(ctx, opts, zap.NewNop(), get)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if tl == nil || !tl.Closed() {
		t.Fatalf("listener not closed")
	}
}

func TestStartTransportError(t *testing.T) {
	get := func(string, transport.Config) (transport.Interface, error) {
		ft := fakeTransport{listen: func(context.Context, string) (net.Listener, error) {
			return nil, errors.New("listen fail")
		}}
		return ft, nil
	}
	opts := Options{Listen: []string{"fake://127.0.0.1:0"}}
	err := start(context.Background(), opts, zap.NewNop(), get)
	if err == nil || err.Error() != "listen \"fake://127.0.0.1:0\": listen fail" {
		t.Fatalf("expected listen error, got %v", err)
	}
}
