package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

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
	modules       map[string]string
	transports    []string
	tcpPort       int
	stdio         bool
	inetd         bool
	tlsCert       string
	tlsKey        string
	caCert        string
	allowInsecure bool
}

func parseModules(vals []string) map[string]string {
	m := make(map[string]string)
	for _, v := range vals {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

func parseOptions(v *viper.Viper) options {
	return options{
		modules:       parseModules(v.GetStringSlice("module")),
		transports:    splitTransports(v.GetString("transport")),
		tcpPort:       v.GetInt("tcp-port"),
		stdio:         v.GetBool("stdio"),
		inetd:         v.GetBool("inetd"),
		tlsCert:       v.GetString("tls-cert"),
		tlsKey:        v.GetString("tls-key"),
		caCert:        v.GetString("ca-cert"),
		allowInsecure: v.GetBool("allow-insecure"),
	}
}

func splitTransports(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	fs := pflag.NewFlagSet("lvmsyncd", pflag.ExitOnError)
	fs.StringSlice("module", nil, "module mapping name=path")
	fs.String("transport", "tcp+tls", "comma-separated transports")
	fs.Int("tcp-port", 8730, "TCP port to listen on")
	fs.Bool("stdio", false, "use stdio for a single connection")
	fs.Bool("inetd", false, "use stdio under inetd activation")
	fs.String("tls-cert", "", "TLS certificate file")
	fs.String("tls-key", "", "TLS key file")
	fs.String("ca-cert", "", "CA certificate file")
	fs.Bool("allow-insecure", false, "allow insecure connections (no TLS)")
	cmd.Flags().AddFlagSet(fs)
	if err := v.BindPFlags(fs); err != nil {
		return err
	}
	v.SetEnvPrefix("LVMSYNCD")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
	return nil
}

func run(args []string, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
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

func start(ctx context.Context, opts options, logger *zap.Logger) error {
	cfg := transport.Config{Logger: logger, AllowInsecure: opts.allowInsecure}
	if !opts.allowInsecure {
		if opts.tlsCert == "" || opts.tlsKey == "" || opts.caCert == "" {
			return fmt.Errorf("tls-cert, tls-key, and ca-cert are required unless --allow-insecure is set")
		}
	} else {
		logger.Warn("allow_insecure_enabled", zap.String("component", "lvmsyncd"))
	}
	if opts.tlsCert != "" && opts.tlsKey != "" {
		cert, err := tls.LoadX509KeyPair(opts.tlsCert, opts.tlsKey)
		if err != nil {
			return fmt.Errorf("load TLS key pair: %w", err)
		}
		cfg.ClientCert = cert
		cfg.ServerCert = cert
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
	trs, err := transport.GetOrdered(opts.transports, cfg)
	if err != nil {
		return fmt.Errorf("get transport: %w", err)
	}
	if opts.stdio || opts.inetd {
		if len(trs) == 0 {
			return fmt.Errorf("no transport specified")
		}
		conn := &stdioConn{}
		return handleConn(ctx, conn, trs[0], opts.modules, logger)
	}
	addr := fmt.Sprintf(":%d", opts.tcpPort)
	errCh := make(chan error, len(trs))
	var wg sync.WaitGroup
	for _, tr := range trs {
		ln, err := tr.Listen(ctx, addr)
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

func handleConn(ctx context.Context, conn net.Conn, tr transport.Interface, modules map[string]string, logger *zap.Logger) error {
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
		os.Exit(1)
	}
}
