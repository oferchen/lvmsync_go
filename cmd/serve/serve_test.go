package serve

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func TestEnvVarPrecedence(t *testing.T) {
	v := viper.New()
	cmd := &cobra.Command{Use: "serve"}
	bindFlags(cmd, v)

	t.Setenv("LVMSYNC_SERVE_TRANSPORT", "envtransport")
	t.Setenv("LVMSYNC_SERVE_QUIC_LISTEN", "envaddr:1234")
	t.Setenv("LVMSYNC_SERVE_TLS_CERT", "envcert.pem")
	t.Setenv("LVMSYNC_SERVE_TLS_KEY", "envkey.pem")
	t.Setenv("LVMSYNC_SERVE_CA_CERT", "envca.pem")
	t.Setenv("LVMSYNC_SERVE_ALLOW_INSECURE", "false")

	args := []string{
		"--transport", "flagtransport",
		"--tls-cert", "flagcert.pem",
		"--allow-insecure",
	}
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	got := Options{
		Transport:     v.GetString("transport"),
		QUICListen:    v.GetString("quic-listen"),
		TLSCert:       v.GetString("tls-cert"),
		TLSKey:        v.GetString("tls-key"),
		CACert:        v.GetString("ca-cert"),
		AllowInsecure: v.GetBool("allow-insecure"),
	}
	want := Options{
		Transport:     "flagtransport",
		QUICListen:    "envaddr:1234",
		TLSCert:       "flagcert.pem",
		TLSKey:        "envkey.pem",
		CACert:        "envca.pem",
		AllowInsecure: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestStartServer(t *testing.T) {
	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := Options{Transport: "quic", QUICListen: "127.0.0.1:0", AllowInsecure: true}
	errCh := make(chan error, 1)
	go func() { errCh <- startServer(ctx, opts, logger) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("startServer: %v", err)
	}
}

func TestStartServerRequiresCerts(t *testing.T) {
	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := Options{Transport: "quic", QUICListen: "127.0.0.1:0"}
	if err := startServer(ctx, opts, logger); err == nil {
		t.Fatalf("expected error for missing certs")
	}
}
