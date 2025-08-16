package serve

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
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
	bindFlags(cmd, v)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func bindFlags(cmd *cobra.Command, v *viper.Viper) {
	fs := pflag.NewFlagSet("serve", pflag.ExitOnError)
	fs.String("transport", "quic", "transport to use")
	fs.String("quic-listen", ":12000", "QUIC listen address")
	fs.String("tls-cert", "", "TLS certificate file")
	fs.String("tls-key", "", "TLS key file")
	fs.String("ca-cert", "", "CA certificate file")
	fs.Bool("allow-insecure", false, "allow insecure (no TLS)")
	cmd.Flags().AddFlagSet(fs)
	v.BindPFlags(fs)
	v.SetEnvPrefix("LVMSYNC_SERVE")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
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
				return
			}
			conn.Close()
		}
	}()
	<-ctx.Done()
	return nil
}
