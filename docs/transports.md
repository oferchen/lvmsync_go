# Transports

## Usage

`lvmsyncd` exposes transports by listening on URIs passed to `--listen`. Each
scheme selects a transport such as `quic`, `tcp+tls`, or `ssh`. Clients choose
their preferred order with the `--transport` flag; transports are tried in order
until a connection is established. Flags override `LVMSYNC_TRANSPORT_*`
environment variables, which override `transport` keys in `config.yaml`.

Each transport constructor accepts a configuration with an optional `zap.Logger`.
When no logger is supplied, a no-op logger is used.

Every session begins with a textual handshake starting with `lvmsync PROTO[3]`.
Tokens advertise supported transports, compression algorithms, digests,
endianness (`endian:<little|big>`), block sizes (`block:<n>`), deduplication
mode (`dedup:<mode>`), content-defined chunking ranges (`cdcmin:<n>`
`cdcavg:<n>` `cdcmax:<n>`), and O_DIRECT capability (`odirect`). The handshake
also carries a resume token (`resume:<token>`) so interrupted sessions can
continue and a maximum in-flight hint (`inflight:<n>`) to negotiate concurrency.


Transport configuration groups flags with pflag, binds them to Viper, and logs connection events with zap.

## Flag Group Example

```go
import (
    "github.com/spf13/pflag"
    "github.com/spf13/viper"
    "go.uber.org/zap"
)

func initTransports() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    fs := pflag.NewFlagSet("transport", pflag.ExitOnError)
    fs.String("transport", "ssh,tcp+tls,h2,quic", "ordered transports")
    fs.Int("tcp-port", 9443, "TCP listener port")

    v := viper.New()
    v.BindPFlags(fs)
}
```

## Security Defaults

Handshake validation rejects mismatched ALPN protocols or TLS versions to ensure
both peers agree on the connection parameters.

TLS-based transports enable mutual TLS by default. Both client and server
explicitly restrict `tls.Config.CipherSuites` to
`TLS_AES_128_GCM_SHA256`, `TLS_AES_256_GCM_SHA384`, and
`TLS_CHACHA20_POLY1305_SHA256`; handshakes fail if peers do not support one of
these ciphers. TLS transports require an explicit set of trusted CA roots.
Connections are rejected if no roots are provided unless the transport
configuration sets `AllowInsecure` and the user explicitly acknowledges the risk
with the `--allow-insecure` flag or `LVMSYNC_ALLOW_INSECURE` environment
variable. This disables certificate verification, logs a warning, and should be
used only in development.

## Examples

Expose multiple transports with `lvmsyncd`:

```sh
lvmsyncd --listen tcp+tls://:9443 --listen ssh://:2222 \
  --server-cert cert.pem --server-key key.pem \
  --client-cert cert.pem --client-key key.pem --ca-cert ca.pem \
  --ssh-host-key-path host_key
```

Listeners enforce these defaults:

- `--keepalive-time` (2m): interval between server pings.
- `--keepalive-timeout` (20s): wait for a ping acknowledgement before closing.
- `--request-timeout` (15s): deadline enforced on unary RPC handlers.

## QUIC

- Uses [quic-go](https://github.com/quic-go/quic-go)
- TLS 1.3 with mutual authentication
- ALPN negotiation using `lvmsync`
- Bidirectional streams and datagram support
- BBR congestion control
- Rejects 0-RTT packets
- Flags: `--server-cert`, `--server-key`, `--client-cert`, `--client-key`, `--ca-cert`, `--allow-insecure`

Example:

```sh
lvmsyncd --listen quic://:12000 --server-cert cert.pem --server-key key.pem --client-cert cert.pem --client-key key.pem --ca-cert ca.pem
```

## HTTP/2 (h2)

- Runs over TLS 1.3 with mutual authentication
- Provides stream-level back-pressure
- Enforces context deadlines during connection and HTTP/2 handshakes
- Flags: `--server-cert`, `--server-key`, `--client-cert`, `--client-key`, `--ca-cert`, `--allow-insecure`, `--tcp-port`

## TCP+TLS

- Plain TCP encapsulated in TLS 1.3
- Requires mutual TLS authentication
- Logs a warning if listener shutdown encounters an error
- Flags: `--server-cert`, `--server-key`, `--client-cert`, `--client-key`, `--ca-cert`, `--allow-insecure`, `--tcp-port`

## SSH

- Establishes sessions using `golang.org/x/crypto/ssh`
- Supports `sudo -n` escalation hooks
- Uses context deadlines for handshake I/O, failing fast when the caller's context times out
- Verifies server host keys using `known_hosts` or an explicit `--ssh-host-key`; unknown hosts are rejected
- Key authentication via `--ssh-key`/`LVMSYNC_SSH_KEY`
- Optional agent auth with `--ssh-agent`/`LVMSYNC_SSH_AGENT` using `SSH_AUTH_SOCK`
- Listeners require a persistent host key via `--ssh-host-key-path` unless `--allow-insecure` is enabled
- Flags: `--ssh-user`, `--ssh-password`, `--ssh-key`, `--ssh-host-key`, `--ssh-host-key-path`, `--ssh-agent`, `--allow-insecure`

## RSYNC

- Uses the rsync daemon protocol for wire compatibility
- Plain TCP; no encryption or authentication
- Interoperates with upstream `rsyncd` implementations (e.g., OpenRSYNC on macOS 15)
- Known issue: macOS 15 ships OpenRSYNC 0.6 which may abort large transfers; verify with upstream rsync if issues arise
- Security: traffic is unencrypted and unauthenticated. `--allow-insecure` must be set and connections should only run on trusted networks or within secure tunnels
- Flags: `--transport rsync` selects this transport and requires `--allow-insecure`
