package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"

	grpcserver "lvmsync_go/grpc/server"
	lvmagent "lvmsync_go/internal/lvm"
)

type failingSyncCore struct {
	zapcore.Core
	err error
}

func (c *failingSyncCore) Sync() error { return c.err }

func TestSyncLoggerLogsError(t *testing.T) {
	syncErr := errors.New("sync failure")
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(&failingSyncCore{Core: core, err: syncErr})
	syncLogger(logger)
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(entries))
	}
	if entries[0].Message != "Logger sync error" {
		t.Fatalf("unexpected log message %q", entries[0].Message)
	}
	if got := entries[0].ContextMap()["error"]; got != syncErr.Error() {
		t.Fatalf("expected error %q, got %v", syncErr.Error(), got)
	}
}

func TestInitConfigPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "grpcd.yaml")
	cfg := []byte("grpc-port: 1111\nallow-insecure: false\n")
	if err := os.WriteFile(cfgFile, cfg, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Run("config file", func(t *testing.T) {
		v, err := initConfig([]string{"--config", cfgFile})
		if err != nil {
			t.Fatalf("initConfig: %v", err)
		}
		if port := v.GetInt("grpc-port"); port != 1111 {
			t.Fatalf("expected port 1111, got %d", port)
		}
		if v.GetBool("allow-insecure") {
			t.Fatalf("expected allow-insecure false")
		}
	})

	t.Run("env overrides", func(t *testing.T) {
		t.Setenv("LVMSYNC_GRPC_GRPC_PORT", "2222")
		t.Setenv("LVMSYNC_GRPC_ALLOW_INSECURE", "true")
		v, err := initConfig([]string{"--config", cfgFile})
		if err != nil {
			t.Fatalf("initConfig: %v", err)
		}
		if port := v.GetInt("grpc-port"); port != 2222 {
			t.Fatalf("expected port 2222, got %d", port)
		}
		if !v.GetBool("allow-insecure") {
			t.Fatalf("expected allow-insecure true")
		}
	})

	t.Run("flag overrides", func(t *testing.T) {
		t.Setenv("LVMSYNC_GRPC_GRPC_PORT", "2222")
		t.Setenv("LVMSYNC_GRPC_ALLOW_INSECURE", "true")
		v, err := initConfig([]string{"--config", cfgFile, "--grpc-port", "3333", "--allow-insecure=false"})
		if err != nil {
			t.Fatalf("initConfig: %v", err)
		}
		if port := v.GetInt("grpc-port"); port != 3333 {
			t.Fatalf("expected port 3333, got %d", port)
		}
		if v.GetBool("allow-insecure") {
			t.Fatalf("expected allow-insecure false")
		}
	})
}

func TestFlagSetsBindToViper(t *testing.T) {
	v, err := initConfig([]string{
		"--grpc-port", "9999",
		"--tls-cert", "cert.pem",
		"--tls-key", "key.pem",
		"--ca-cert", "ca.pem",
		"--allow-insecure",
	})
	if err != nil {
		t.Fatalf("initConfig: %v", err)
	}
	if port := v.GetInt("grpc-port"); port != 9999 {
		t.Fatalf("expected grpc-port 9999, got %d", port)
	}
	if cert := v.GetString("tls-cert"); cert != "cert.pem" {
		t.Fatalf("expected tls-cert cert.pem, got %q", cert)
	}
	if key := v.GetString("tls-key"); key != "key.pem" {
		t.Fatalf("expected tls-key key.pem, got %q", key)
	}
	if ca := v.GetString("ca-cert"); ca != "ca.pem" {
		t.Fatalf("expected ca-cert ca.pem, got %q", ca)
	}
	if insecure := v.GetBool("allow-insecure"); !insecure {
		t.Fatalf("expected allow-insecure true")
	}
}

func TestInitConfigUnknownFlag(t *testing.T) {
	if _, err := initConfig([]string{"--unknown"}); err == nil {
		t.Fatalf("expected error for unknown flag")
	}
}

func TestInitConfigInvalidConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "grpcd.yaml")
	// Write malformed YAML
	if err := os.WriteFile(cfgFile, []byte(":-bad"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := initConfig([]string{"--config", cfgFile}); err == nil {
		t.Fatalf("expected error for malformed config")
	}
}

func TestMainLogsErrorAndExits(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"grpcd", "--tls-cert", "missing", "--tls-key", "missing"}
	defer func() { os.Args = oldArgs }()

	var exitCode int
	exitFunc = func(code int) { exitCode = code; panic("exit") }
	defer func() { exitFunc = os.Exit }()

	core, logs := observer.New(zap.ErrorLevel)
	newZapProduction = func(opts ...zap.Option) (*zap.Logger, error) { return zap.New(core, opts...), nil }
	defer func() { newZapProduction = zap.NewProduction }()

	defer func() {
		if r := recover(); r != "exit" {
			t.Fatalf("unexpected panic: %v", r)
		}
		if exitCode != 1 {
			t.Fatalf("expected exit code 1, got %d", exitCode)
		}
		entries := logs.All()
		if len(entries) == 0 {
			t.Fatalf("expected log entry")
		}
		if entries[0].Level != zap.ErrorLevel {
			t.Fatalf("expected error level, got %v", entries[0].Level)
		}
		if entries[0].Message != "init gRPC server" {
			t.Fatalf("unexpected message %q", entries[0].Message)
		}
	}()

type fakeListener struct{}

func (fakeListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (fakeListener) Close() error              { return nil }
func (fakeListener) Addr() net.Addr            { return &net.TCPAddr{} }

func TestMainErrorPropagation(t *testing.T) {
	tests := []struct {
		name    string
		setup   func()
		wantMsg string
	}{
		{
			name: "server error",
			setup: func() {
				newServer = func(cfg grpcserver.Config, a lvmagent.Agent) (*grpc.Server, error) {
					return nil, errors.New("srv err")
				}
			},
			wantMsg: "init gRPC server",
		},
		{
			name: "listen error",
			setup: func() {
				newServer = func(cfg grpcserver.Config, a lvmagent.Agent) (*grpc.Server, error) {
					return grpc.NewServer(), nil
				}
				listen = func(network, addr string) (net.Listener, error) {
					return nil, errors.New("listen err")
				}
			},
			wantMsg: "listen",
		},
		{
			name: "serve error",
			setup: func() {
				newServer = func(cfg grpcserver.Config, a lvmagent.Agent) (*grpc.Server, error) {
					return grpc.NewServer(), nil
				}
				listen = func(network, addr string) (net.Listener, error) {
					return fakeListener{}, nil
				}
				serve = func(s *grpc.Server, lis net.Listener) error {
					return errors.New("serve err")
				}
			},
			wantMsg: "serve",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				newZapProduction = zap.NewProduction
				newServer = grpcserver.New
				listen = net.Listen
				serve = func(s *grpc.Server, lis net.Listener) error { return s.Serve(lis) }
				exitFunc = os.Exit
				os.Args = []string{"grpcd"}
			}()

			core, logs := observer.New(zap.ErrorLevel)
			newZapProduction = func(...zap.Option) (*zap.Logger, error) { return zap.New(core), nil }

			var code int
			exitFunc = func(c int) { code = c }

			os.Args = []string{"grpcd"}
			tt.setup()
			main()

			if code != 1 {
				t.Fatalf("expected exit code 1, got %d", code)
			}
			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("expected one log entry, got %d", len(entries))
			}
			if entries[0].Message != tt.wantMsg {
				t.Fatalf("expected log message %q, got %q", tt.wantMsg, entries[0].Message)
			}
		})
	}
}
