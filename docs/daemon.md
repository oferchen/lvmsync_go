# Daemon

LVMSync ships a standalone daemon named `lvmsyncd` for accepting transfer requests and dispatching work to optional modules.

## Module configuration and ACLs

The daemon loads modules declared in the configuration file or via repeated
`--module` flags.  Each module entry includes the shared object path and an
optional access control list (ACL) restricting which clients may invoke it.

```yaml
module:
  - path: /usr/lib/lvmsync/modules/snapshot.so
    acl:
      # Allow clients with these certificate subjects
      - subject: backup@example.com
      # CIDR blocks permitted to use this module
      - cidr: 192.0.2.0/24
  - path: ./debug.so
    # Empty ACL allows all authenticated clients
    acl: []
```

ACL checks apply to the client's authenticated identity derived from mutual
TLS (mTLS).  Unknown or unauthorised identities are rejected before the module
initialises.

## Listener options

`lvmsyncd` listens on one or more URIs supplied with `--listen` or the
`LVMSYNC_DAEMON_LISTEN` environment variable.  The daemon also supports two
special listeners:

- `--stdio` – serve a single connection over standard input/output.
- `--inetd` – inherit an already‑bound socket from the parent process.

Transport security uses TLS by default.  Provide certificate paths with
`--tls-cert`, `--tls-key`, and `--ca-cert`.  Insecure mode can be enabled with
`--allow-insecure`, but it should only be used for local development.

Example listeners:

```sh
# Multiple network listeners
lvmsyncd --listen unix:///run/lvmsyncd.sock --listen tcp://:9443

# Systemd socket activation
lvmsyncd --inetd

# Standard input/output
lvmsyncd --stdio
```

## Security requirements

Connections must satisfy the following requirements:

- **mTLS** – both sides present certificates signed by the same CA.  Clients are
  matched against module ACLs using the certificate subject or SAN.
- **Host key checks** – when modules initiate SSH connections, the remote host's
  key must match an entry in `known_hosts`.
- **Certificate validation** – certificates are checked for expiry and
  revocation.

Example mTLS invocation:

```sh
lvmsyncd \
  --listen tcp://:9443 \
  --tls-cert server.pem --tls-key server.key \
  --ca-cert ca.pem --module /usr/lib/lvmsync/modules/snapshot.so
```

This configuration accepts clients on port 9443, requires mutual TLS, and loads
a snapshot module restricted by its ACL rules.

