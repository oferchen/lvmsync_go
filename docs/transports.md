# Transports

LVMSync supports multiple transports selectable with the `--transport` flag.
Transports are tried in order until a connection is established.

Default order: `quic,h2,tcp+tls,ssh`.

Each transport begins with a protocol handshake exchanging supported features.
The handshake now includes a resume token (`resume:<token>`) to continue
interrupted sessions and a maximum in-flight hint (`inflight:<n>`) to negotiate
concurrency.

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
