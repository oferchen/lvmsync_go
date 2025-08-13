# Transports

LVMSync supports multiple transports selected with the `--transport` flag.

## Handshake Negotiation

During connection setup the sender and receiver exchange:

- `sector_size`
- `alignment`
- `max_concurrency`
- deduplication and compression capabilities

This negotiation ensures both sides agree on block sizes and enabled features before data moves.

## Security Defaults

- `tcp+tls` requires mutual TLS with `--tls_cert`, `--tls_key`, and a trusted `--ca_cert`.
- `ssh` relies on user keys and host key verification; mTLS does not apply.

## Flags

| Flag | Environment variable | Description | mTLS |
|------|----------------------|-------------|------|
| `--transport` | `LVMSYNC_TRANSPORT` | Ordered transports to try (e.g., `tcp+tls,ssh`) | n/a |
| `--concurrency` | `LVMSYNC_CONCURRENCY` | Stream concurrency (0 to autotune based on BDP) | n/a |
| `--tcp_port` | `LVMSYNC_TCP_PORT` | TCP+TLS port | ✅ |
| `--ssh_port` | `LVMSYNC_SSH_PORT` | SSH port | ❌ |
| `--tls_cert` | `LVMSYNC_TLS_CERT` | TLS certificate file | ✅ |
| `--tls_key` | `LVMSYNC_TLS_KEY` | TLS key file | ✅ |
| `--ca_cert` | `LVMSYNC_CA_CERT` | CA certificate file | ✅ |
| `--tcp_parallel` | `LVMSYNC_TCP_PARALLEL` | Number of parallel TCP connections | n/a |
| `--tcp_lowat` | `LVMSYNC_TCP_LOWAT` | TCP_NOTSENT_LOWAT in bytes | n/a |
