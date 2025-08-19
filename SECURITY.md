# Security Model

LVMSync splits privileged and unprivileged responsibilities. The main controller runs as a regular user and delegates any operation that requires elevated permissions to a small, audited helper.

## Non-root controller and helper

The controller orchestrates replication, validates parameters, and interacts with remote peers. Operations that require root — such as managing LVM volumes, opening raw block devices or issuing discard commands — are executed through a privileged helper invoked via `sudo` or Linux capabilities. The helper performs only the requested action and exits immediately, minimising the trusted code surface.

## Probe and verify modes

Read-only operations can run without elevated privileges.  
`--probe-only` checks device metadata, validates privileges, and emits dry-run estimates.  
`--verify-only` reads source and destination devices and reports mismatches.  
Both commands exit `0` on success, return `60` for verification failures, and `10` when required capabilities are missing.

## Example sudoers configuration

Tight `sudoers` rules limit what the helper may execute. See [docs/sudoers.md](docs/sudoers.md) for command-specific entries:

```sudoers
# Allow LVM administration commands
lvmsync ALL=(root) NOPASSWD: /sbin/lvm, /sbin/lvcreate, /sbin/lvremove, /sbin/lvs, /sbin/pvs, /sbin/vgs

# Permit opening devices and issuing writes or discards through the helper
lvmsync ALL=(root) NOPASSWD: \
    /usr/local/bin/lvmsync-helper open, \
    /usr/local/bin/lvmsync-helper write, \
    /usr/local/bin/lvmsync-helper discard

# Enable direct blkdiscard when the helper is unavailable
lvmsync ALL=(root) NOPASSWD: /usr/sbin/blkdiscard
```

Adjust paths to match your distribution.

## Environment sanitization

The helper normally inherits the caller's environment, including `PATH` and
potentially unsafe variables such as `LD_PRELOAD` or `GCONV_PATH`. Pass the
`--sanitize-env` flag or enable the `SanitizeEnv` option to run the helper with
a minimal, whitelisted environment that drops those variables and enforces a
safe `PATH` (`/usr/sbin:/usr/bin:/sbin:/bin`). Sanitization is disabled by
default to avoid surprising behavior in mixed environments.

## Risks of raw-device writes

Granting raw-device access lets the helper overwrite any block on the target. A misconfigured path or bug could destroy unrelated data. LVMSync mitigates this risk by:

* Requiring explicit device paths; globbing and symlinks are rejected.
* Verifying device size and LVM signatures before writing.
* Dropping privileges immediately after completing the privileged section.

Review `sudoers` entries carefully and test on non-production systems before granting wide access.
