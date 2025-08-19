#!/usr/bin/env bash
set -euo pipefail

TMPDIR=$(mktemp -d)
PORT=$(shuf -i 20000-65000 -n1)
cleanup() {
  set +e
  if [ -n "${SERVER_PID:-}" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

# Generate CA
openssl req -x509 -newkey rsa:2048 -days 1 -nodes \
  -keyout "$TMPDIR/ca.key" -out "$TMPDIR/ca.crt" \
  -subj "/CN=Test CA" >/dev/null 2>&1

# Server certificate with SAN
cat >"$TMPDIR/openssl.cnf" <<'CFG'
[req]
distinguished_name = req_distinguished_name
prompt = no
[req_distinguished_name]
CN = localhost
[v3_req]
subjectAltName = DNS:localhost
CFG
openssl req -new -newkey rsa:2048 -nodes -keyout "$TMPDIR/server.key" \
  -out "$TMPDIR/server.csr" -config "$TMPDIR/openssl.cnf" >/dev/null 2>&1
openssl x509 -req -in "$TMPDIR/server.csr" -CA "$TMPDIR/ca.crt" \
  -CAkey "$TMPDIR/ca.key" -CAcreateserial -days 1 \
  -out "$TMPDIR/server.crt" -extensions v3_req -extfile "$TMPDIR/openssl.cnf" >/dev/null 2>&1

# Client certificate
openssl req -newkey rsa:2048 -nodes -keyout "$TMPDIR/client.key" \
  -out "$TMPDIR/client.csr" -subj "/CN=test-client" >/dev/null 2>&1
openssl x509 -req -in "$TMPDIR/client.csr" -CA "$TMPDIR/ca.crt" \
  -CAkey "$TMPDIR/ca.key" -CAcreateserial -days 1 \
  -out "$TMPDIR/client.crt" >/dev/null 2>&1

# Build binaries
BIN_D="$TMPDIR/lvmsyncd"
go build -o "$BIN_D" ./cmd/lvmsyncd
cat > "$TMPDIR/client.go" <<'EOG'
package main

import (
  "context"
  "crypto/tls"
  "crypto/x509"
  "flag"
  "fmt"
  "io"
  "os"

  "lvmsync_go/common"
  "lvmsync_go/transport"
  _ "lvmsync_go/transport/tcp_tls"
  "go.uber.org/zap"
)

func main() {
  addr := flag.String("addr", "localhost:0", "address")
  ca := flag.String("ca", "", "CA cert")
  certPath := flag.String("cert", "", "client cert")
  keyPath := flag.String("key", "", "client key")
  serverCertPath := flag.String("server-cert", "", "server cert")
  serverKeyPath := flag.String("server-key", "", "server key")
  allowInsecure := flag.Bool("allow-insecure", false, "allow insecure")
  module := flag.String("module", "test", "module name")
  flag.Parse()

  cfg := transport.Config{AllowInsecure: *allowInsecure, Logger: zap.NewNop()}
  if *ca != "" {
    pem, err := os.ReadFile(*ca)
    if err != nil {
      fmt.Fprintf(os.Stderr, "read ca: %v\n", err)
      os.Exit(1)
    }
    roots := x509.NewCertPool()
    if !roots.AppendCertsFromPEM(pem) {
      fmt.Fprintln(os.Stderr, "invalid ca")
      os.Exit(1)
    }
    cfg.Roots = roots
  }
  if *serverCertPath != "" && *serverKeyPath != "" {
    cert, err := tls.LoadX509KeyPair(*serverCertPath, *serverKeyPath)
    if err != nil {
      fmt.Fprintf(os.Stderr, "load server cert: %v\n", err)
      os.Exit(1)
    }
    cfg.ServerCert = cert
  }
  if *certPath != "" && *keyPath != "" {
    cert, err := tls.LoadX509KeyPair(*certPath, *keyPath)
    if err != nil {
      fmt.Fprintf(os.Stderr, "load client cert: %v\n", err)
      os.Exit(1)
    }
    cfg.ClientCert = cert
  }
  tr, err := transport.Get("tcp+tls", cfg)
  if err != nil {
    fmt.Fprintf(os.Stderr, "get transport: %v\n", err)
    os.Exit(1)
  }
  ctx := context.Background()
  conn, err := tr.Dial(ctx, *addr)
  if err != nil {
    fmt.Fprintf(os.Stderr, "dial: %v\n", err)
    os.Exit(1)
  }
  defer conn.Close()
  if _, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{}); err != nil {
    fmt.Fprintf(os.Stderr, "negotiate: %v\n", err)
    os.Exit(1)
  }
  if _, err := io.WriteString(conn, *module+"\n"); err != nil {
    fmt.Fprintf(os.Stderr, "write module: %v\n", err)
    os.Exit(1)
  }
}
EOG

go build -o "$TMPDIR/client" "$TMPDIR/client.go"

# Start daemon
"$BIN_D" --listen "tcp+tls://127.0.0.1:$PORT" \
  --server-cert "$TMPDIR/server.crt" --server-key "$TMPDIR/server.key" \
  --client-cert "$TMPDIR/client.crt" --client-key "$TMPDIR/client.key" \
  --ca-cert "$TMPDIR/ca.crt" --module test &
SERVER_PID=$!

sleep 1

# Positive case
"$TMPDIR/client" -addr "localhost:$PORT" -cert "$TMPDIR/client.crt" -key "$TMPDIR/client.key" -ca "$TMPDIR/ca.crt" -server-cert "$TMPDIR/server.crt" -server-key "$TMPDIR/server.key" -module test

# Negative case: missing client cert
if "$TMPDIR/client" -addr "localhost:$PORT" -ca "$TMPDIR/ca.crt" -server-cert "$TMPDIR/server.crt" -server-key "$TMPDIR/server.key"; then
  echo "unexpected success without client cert" >&2
  exit 1
fi

exit 0
