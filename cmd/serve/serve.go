package serve

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/common"
	grpcserver "lvmsync_go/grpc/server"
	"lvmsync_go/transport"
	_ "lvmsync_go/transport/quic"
)

// Options holds configuration for the serve command.
type Options struct {
	Transport     string
	QUICListen    string
	TLSCert       string
	TLSKey        string
	CACert        string
	AllowInsecure bool
}

// Run executes the serve command with provided arguments and logger.
// Args should exclude the "serve" subcommand itself.
func Run(args []string, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	defer rootcmd.SyncLogger(logger)

	v := viper.New()
	cmd := &cobra.Command{
		Use:   "serve [flags]",
		Short: "Run a transport listener",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := Options{
				Transport:     v.GetString("transport"),
				QUICListen:    v.GetString("quic-listen"),
				TLSCert:       v.GetString("tls-cert"),
				TLSKey:        v.GetString("tls-key"),
				CACert:        v.GetString("ca-cert"),
				AllowInsecure: v.GetBool("allow-insecure"),
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return startServer(ctx, opts, logger)
		},
	}
	if err := bindFlags(cmd, v); err != nil {
		return err
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

type flagBinder interface {
	BindPFlags(*pflag.FlagSet) error
	SetEnvPrefix(string)
	SetEnvKeyReplacer(*strings.Replacer)
	AutomaticEnv()
}

func bindFlags(cmd *cobra.Command, v flagBinder) error {
	fs := pflag.NewFlagSet("serve", pflag.ExitOnError)
	fs.String("transport", "quic", "transport to use")
	fs.String("quic-listen", ":12000", "QUIC listen address")
	fs.String("tls-cert", "", "TLS certificate file")
	fs.String("tls-key", "", "TLS key file")
	fs.String("ca-cert", "", "CA certificate file")
	fs.Bool("allow-insecure", false, "allow insecure (no TLS)")
	cmd.Flags().AddFlagSet(fs)
	if err := v.BindPFlags(fs); err != nil {
		return err
	}
	v.SetEnvPrefix("LVMSYNC_SERVE")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
	return nil
}

func startServer(ctx context.Context, opts Options, logger *zap.Logger) error {
	cfg := transport.Config{Logger: logger, AllowInsecure: opts.AllowInsecure}
	if !opts.AllowInsecure {
		if opts.TLSCert == "" || opts.TLSKey == "" || opts.CACert == "" {
			return fmt.Errorf("tls-cert, tls-key, and ca-cert are required unless --allow-insecure is set")
		}
	}
	if opts.TLSCert != "" && opts.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(opts.TLSCert, opts.TLSKey)
		if err != nil {
			return fmt.Errorf("load TLS key pair: %w", err)
		}
		cfg.ClientCert = cert
	}
	if opts.CACert != "" {
		pem, err := os.ReadFile(opts.CACert)
		if err != nil {
			return fmt.Errorf("read CA cert: %w", err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return fmt.Errorf("invalid CA cert")
		}
		cfg.Roots = roots
	}
	tr, err := transport.Get(opts.Transport, cfg)
	if err != nil {
		return fmt.Errorf("get transport: %w", err)
	}
	ln, err := tr.Listen(ctx, opts.QUICListen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Error("accept_failed", zap.Error(err))
				return
			}
			go handleConn(ctx, conn, tr, opts, logger)
		}
	}()

	<-ctx.Done()
	ln.Close()
	return nil
}

type singleConnListener struct {
	conn net.Conn
	ch   chan net.Conn
	once sync.Once
}

func newSingleConnListener(c net.Conn) *singleConnListener {
	ch := make(chan net.Conn, 1)
	ch <- c
	return &singleConnListener{conn: c, ch: ch}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	c, ok := <-l.ch
	if !ok {
		return nil, net.ErrClosed
	}
	return c, nil
}

func (l *singleConnListener) Close() error {
	var err error
	l.once.Do(func() {
		close(l.ch)
		err = l.conn.Close()
	})
	return err
}

func (l *singleConnListener) Addr() net.Addr { return l.conn.LocalAddr() }

func handleConn(ctx context.Context, conn net.Conn, tr transport.Interface, opts Options, logger *zap.Logger) {
	remote := conn.RemoteAddr().String()
	logger.Info("conn_accepted", zap.String("remote_addr", remote))
	defer func() {
		_ = conn.Close()
		logger.Info("conn_closed", zap.String("remote_addr", remote))
	}()

	if _, err := tr.Negotiate(ctx, conn, transport.Server, common.Handshake{}); err != nil {
		logger.Error("handshake_failed", zap.String("remote_addr", remote), zap.Error(err))
		return
	}
	logger.Info("handshake_succeeded", zap.String("remote_addr", remote))

	srvCfg := grpcserver.Config{
		TLSCert:       opts.TLSCert,
		TLSKey:        opts.TLSKey,
		CACert:        opts.CACert,
		AllowInsecure: opts.AllowInsecure,
	}
	srv, cleanup, err := grpcserver.New(srvCfg, nil, logger)
	if err != nil {
		logger.Error("grpc_init_failed", zap.Error(err))
		return
	}
	l := newSingleConnListener(conn)
	go func() {
		<-ctx.Done()
		l.Close()
		srv.GracefulStop()
	}()
	if err := srv.Serve(l); err != nil && err != net.ErrClosed {
		logger.Error("grpc_serve_error", zap.Error(err))
	}
	cleanup()
}
