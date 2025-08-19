package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/activation"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/common"
	"lvmsync_go/transport"
	_ "lvmsync_go/transport/h2"
	_ "lvmsync_go/transport/quic"
	_ "lvmsync_go/transport/tcp_tls"
)

type options struct {
	modules       map[string]struct{}
	listens       []string
	stdio         bool
	inetd         bool
	serverCert    string
	serverKey     string
	clientCert    string
	clientKey     string
	caCert        string
	allowInsecure bool
}

func parseModules(vals []string) map[string]struct{} {
	m := make(map[string]struct{}, len(vals))
	for _, v := range vals {
		if v != "" {
			m[v] = struct{}{}
		}
	}
	return m
}

func parseOptions(v *viper.Viper) options {
	return options{
		modules:       parseModules(v.GetStringSlice("module")),
		listens:       v.GetStringSlice("listen"),
		stdio:         v.GetBool("stdio"),
		inetd:         v.GetBool("inetd"),
		serverCert:    v.GetString("server-cert"),
		serverKey:     v.GetString("server-key"),
		clientCert:    v.GetString("client-cert"),
		clientKey:     v.GetString("client-key"),
		caCert:        v.GetString("ca-cert"),
		allowInsecure: v.GetBool("allow-insecure"),
	}
}

func bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	fs := pflag.NewFlagSet("lvmsyncd", pflag.ExitOnError)
	fs.StringSlice("module", nil, "module path")
	fs.StringSlice("listen", nil, "transport listen URI")
	fs.Bool("stdio", false, "serve a single connection over stdio")
	fs.Bool("inetd", false, "use stdio under inetd activation")
	fs.String("server-cert", "", "server TLS certificate file")
	fs.String("server-key", "", "server TLS key file")
	fs.String("client-cert", "", "client TLS certificate file")
	fs.String("client-key", "", "client TLS key file")
	fs.String("ca-cert", "", "CA certificate file")
	fs.Bool("allow-insecure", false, "allow insecure connections (no TLS)")
	cmd.Flags().AddFlagSet(fs)
	if err := v.BindPFlags(fs); err != nil {
		return err
	}
	v.SetEnvPrefix("LVMSYNC_DAEMON")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
	return nil
}

func run(args []string, logger *zap.Logger) error {
	if logger == nil {
		return fmt.Errorf("nil logger")
	}
	defer rootcmd.SyncLogger(logger)

	v := viper.New()
	cmd := &cobra.Command{
		Use:   "lvmsyncd",
		Short: "Run lvmsync daemon",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := parseOptions(v)
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return start(ctx, opts, logger)
		},
	}
	if err := bindFlags(cmd, v); err != nil {
		return err
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

type stdioConn struct{}

func (s *stdioConn) Read(p []byte) (int, error)         { return os.Stdin.Read(p) }
func (s *stdioConn) Write(p []byte) (int, error)        { return os.Stdout.Write(p) }
func (s *stdioConn) Close() error                       { return nil }
func (s *stdioConn) LocalAddr() net.Addr                { return dummyAddr("stdio") }
func (s *stdioConn) RemoteAddr() net.Addr               { return dummyAddr("stdio") }
func (s *stdioConn) SetDeadline(t time.Time) error      { return nil }
func (s *stdioConn) SetReadDeadline(t time.Time) error  { return nil }
func (s *stdioConn) SetWriteDeadline(t time.Time) error { return nil }

type dummyAddr string

func (d dummyAddr) Network() string { return string(d) }
func (d dummyAddr) String() string  { return string(d) }

type listenSpec struct {
	scheme string
	addr   string
}

func parseListen(listens []string) ([]listenSpec, error) {
	specs := make([]listenSpec, 0, len(listens))
	for _, l := range listens {
		if l == "" {
			continue
		}
		u, err := url.Parse(l)
		if err != nil {
			return nil, fmt.Errorf("parse listen URI %q: %w", l, err)
		}
		if u.Scheme == "" {
			return nil, fmt.Errorf("missing scheme in %q", l)
		}
		addr := u.Host
		if addr == "" {
			addr = u.Path
		}
		specs = append(specs, listenSpec{scheme: u.Scheme, addr: addr})
	}
	return specs, nil
}

func start(ctx context.Context, opts options, logger *zap.Logger) error {
	cfg := transport.Config{Logger: logger, AllowInsecure: opts.allowInsecure}
	if !opts.allowInsecure {
		if opts.serverCert == "" || opts.serverKey == "" || opts.clientCert == "" || opts.clientKey == "" || opts.caCert == "" {
			return fmt.Errorf("server-cert, server-key, client-cert, client-key, and ca-cert are required unless --allow-insecure is set")
		}
	} else {
		logger.Warn("allow_insecure_enabled", zap.String("component", "lvmsyncd"))
	}
	if opts.serverCert != "" && opts.serverKey != "" {
		cert, err := tls.LoadX509KeyPair(opts.serverCert, opts.serverKey)
		if err != nil {
			return fmt.Errorf("load server TLS key pair: %w", err)
		}
		cfg.ServerCert = cert
	}
	if opts.clientCert != "" && opts.clientKey != "" {
		cert, err := tls.LoadX509KeyPair(opts.clientCert, opts.clientKey)
		if err != nil {
			return fmt.Errorf("load client TLS key pair: %w", err)
		}
		cfg.ClientCert = cert
	}
	if opts.caCert != "" {
		pem, err := os.ReadFile(opts.caCert)
		if err != nil {
			return fmt.Errorf("read CA cert: %w", err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return fmt.Errorf("invalid CA cert")
		}
		cfg.Roots = roots
	}
	specs, err := parseListen(opts.listens)
	if err != nil {
		return err
	}
	if opts.stdio || opts.inetd {
		scheme := "tcp+tls"
		if len(specs) > 0 {
			scheme = specs[0].scheme
		}
		tr, err := transport.Get(scheme, cfg)
		if err != nil {
			return fmt.Errorf("get transport: %w", err)
		}
		conn := &stdioConn{}
		return handleConn(ctx, conn, tr, opts.modules, logger)
	}
	if len(specs) == 0 {
		listeners, err := activation.Listeners()
		if err != nil {
			return fmt.Errorf("activation listeners: %w", err)
		}
		if len(listeners) > 0 {
			tr, err := transport.Get("tcp+tls", cfg)
			if err != nil {
				return fmt.Errorf("get transport: %w", err)
			}
			errCh := make(chan error, len(listeners))
			var wg sync.WaitGroup
			for _, ln := range listeners {
				wg.Add(1)
				go func(ln net.Listener) {
					defer wg.Done()
					for {
						c, err := ln.Accept()
						if err != nil {
							if ctx.Err() != nil {
								return
							}
							errCh <- err
							return
						}
						go handleConn(ctx, c, tr, opts.modules, logger)
					}
				}(ln)
			}
			daemon.SdNotify(false, daemon.SdNotifyReady)
			select {
			case <-ctx.Done():
				wg.Wait()
				daemon.SdNotify(false, daemon.SdNotifyStopping)
				return ctx.Err()
			case err := <-errCh:
				return err
			}
		}
		return fmt.Errorf("no listeners provided")
	}
	errCh := make(chan error, len(specs))
	var wg sync.WaitGroup
	for _, sp := range specs {
		tr, err := transport.Get(sp.scheme, cfg)
		if err != nil {
			return fmt.Errorf("get transport: %w", err)
		}
		if sp.addr == "" {
			return fmt.Errorf("missing address for %s", sp.scheme)
		}
		ln, err := tr.Listen(ctx, sp.addr)
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		wg.Add(1)
		go func(tr transport.Interface, ln net.Listener) {
			defer wg.Done()
			for {
				c, err := ln.Accept()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					errCh <- err
					return
				}
				go handleConn(ctx, c, tr, opts.modules, logger)
			}
		}(tr, ln)
	}
	select {
	case <-ctx.Done():
		wg.Wait()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func handleConn(ctx context.Context, conn net.Conn, tr transport.Interface, modules map[string]struct{}, logger *zap.Logger) error {
	remote := conn.RemoteAddr().String()
	logger.Info("conn_accepted", zap.String("remote_addr", remote))
	defer func() {
		_ = conn.Close()
		logger.Info("conn_closed", zap.String("remote_addr", remote))
	}()
	if _, err := tr.Negotiate(ctx, conn, transport.Server, common.Handshake{}); err != nil {
		logger.Error("handshake_failed", zap.String("remote_addr", remote), zap.Error(err))
		return err
	}
	reader := bufio.NewReader(conn)
	mod, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	mod = strings.TrimSpace(mod)
	if _, ok := modules[mod]; !ok {
		logger.Warn("module_denied", zap.String("module", mod))
		return fmt.Errorf("unauthorized module: %s", mod)
	}
	logger.Info("module_authorized", zap.String("module", mod))
	return nil
}

func main() {
	logger, _ := zap.NewProduction()
	if err := run(os.Args[1:], logger); err != nil {
		os.Exit(rootcmd.ExitCode(err))
	}
}
