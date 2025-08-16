# Transports

## Usage

LVMSync supports multiple transports selectable with the `--transport` flag.
Transports are tried in order until a connection is established. The default
order is `quic,h2,tcp+tls,ssh`.

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
    fs.String("transport", "quic,h2,tcp+tls,ssh", "ordered transports")
    fs.Int("tcp-port", 9443, "TCP listener port")

    v := viper.New()
    v.BindPFlags(fs)
}
```

## Security Defaults

Handshake validation rejects mismatched ALPN protocols or TLS versions to ensure
both peers agree on the connection parameters.

TLS-based transports enable mutual TLS by default and restrict cipher suites to
`TLS_AES_128_GCM_SHA256`, `TLS_AES_256_GCM_SHA384`, and
`TLS_CHACHA20_POLY1305_SHA256`. TLS transports require an explicit set of
trusted CA roots. Connections are rejected if no roots are provided unless the
transport configuration sets `AllowInsecure` to skip verification. Enabling this
option logs a warning.

## Examples

```sh
lvmsync --transport quic,h2,tcp+tls,ssh --tcp-port 9443
```

## gRPC keepalive and timeouts

The gRPC control plane uses keepalive pings and deadlines to detect stalled
clients. Defaults:

- `--keepalive-time` (2m): interval between server pings.
- `--keepalive-timeout` (20s): wait for a ping acknowledgement before closing.
- `--request-timeout` (15s): deadline enforced on unary RPC handlers.

## QUIC

- Uses [quic-go](https://github.com/quic-go/quic-go)
- TLS 1.3 with mutual authentication
- ALPN negotiation using `lvmsync`
- Bidirectional streams and datagram support
- BBR congestion control
- Flags: `--tls_cert`, `--tls_key`, `--ca_cert`, `--allow_insecure`

Example:

```sh
lvmsync --transport quic --tls_cert cert.pem --tls_key key.pem --ca_cert ca.pem
```

Run a listener with the `serve` subcommand:

```sh
lvmsync serve --transport quic --quic-listen :12000 --tls-cert cert.pem --tls-key key.pem --ca-cert ca.pem
```

## HTTP/2 (h2)

- Runs over TLS 1.3 with mutual authentication
- Provides stream-level back-pressure
- Enforces context deadlines during connection and HTTP/2 handshakes
- Flags: `--tls_cert`, `--tls_key`, `--ca_cert`, `--allow_insecure`, `--tcp_port`

## TCP+TLS

- Plain TCP encapsulated in TLS 1.3
- Requires mutual TLS authentication
- Logs a warning if listener shutdown encounters an error
- Flags: `--tls_cert`, `--tls_key`, `--ca_cert`, `--allow_insecure`, `--tcp_port`

## SSH

- Establishes sessions using `golang.org/x/crypto/ssh`
- Supports `sudo -n` escalation hooks
- Uses context deadlines for handshake I/O, failing fast when the caller's context times out
- Verifies server host keys using `known_hosts` or an explicit `--ssh_host_key`; unknown hosts are rejected
- Key authentication via `--ssh_key`/`LVMSYNC_SSH_KEY`
- Optional agent auth with `--ssh_agent`/`LVMSYNC_SSH_AGENT` using `SSH_AUTH_SOCK`
- Flags: `--ssh_user`, `--ssh_password`, `--ssh_key`, `--ssh_host_key`, `--ssh_host_key_path`, `--ssh_agent`, `--allow_insecure`
