# Transports

LVMSync supports multiple transports selectable with the `--transport` flag.
Transports are tried in order until a connection is established.

Default order: `quic,h2,tcp+tls,ssh`.

Every session begins with a textual handshake starting with
`lvmsync PROTO[3]`. Tokens advertise supported transports, compression
algorithms and digests. The handshake also carries a resume token
(`resume:<token>`) so interrupted sessions can continue and a maximum
in‑flight hint (`inflight:<n>`) to negotiate concurrency.

TLS based transports enable mutual TLS by default and restrict cipher suites to
`TLS_AES_128_GCM_SHA256`, `TLS_AES_256_GCM_SHA384`, and
`TLS_CHACHA20_POLY1305_SHA256`.

TLS transports require an explicit set of trusted CA roots. Connections are
rejected if no roots are provided unless the transport configuration sets
`AllowInsecure` to skip verification.

## QUIC

- Uses [quic-go](https://github.com/quic-go/quic-go)
- TLS 1.3 with mutual authentication
- ALPN negotiation using `lvmsync`
- Bidirectional streams and datagram support
- BBR congestion control

Example:

```sh
lvmsync --transport quic --tls_cert cert.pem --tls_key key.pem --ca_cert ca.pem
```

## HTTP/2 (h2)

- Runs over TLS 1.3 with mutual authentication
- Provides stream-level back-pressure

## TCP+TLS

- Plain TCP encapsulated in TLS 1.3
- Requires mutual TLS authentication

## SSH

- Establishes sessions using `golang.org/x/crypto/ssh`
- Supports `sudo -n` escalation hooks

Example selecting transports and custom port:

```sh
lvmsync --transport quic,h2,tcp+tls,ssh --tcp-port 9443
```
