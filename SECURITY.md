# Security Model

LVMSync splits privileged and unprivileged responsibilities. The main controller runs as a regular user and delegates any operation that requires elevated permissions to a small, audited helper.

## Non-root controller and helper

The controller orchestrates replication, validates parameters, and interacts with remote peers. Operations that require root — such as managing LVM volumes, opening raw block devices or issuing discard commands — are executed through a privileged helper invoked via `sudo` or Linux capabilities. The helper performs only the requested action and exits immediately, minimising the trusted code surface.

## Probe and verify modes

Read-only operations can run without elevated privileges.
`--probe-only` checks device metadata, validates privileges, and prints `size_bytes device_id manifest_epoch` so operators can capture the identity tuple before writing.
`--verify-only` reads source and destination devices and reports mismatches.
Both commands exit `0` on success, return `60` for verification failures, and `10` when required capabilities are missing.

Example `--probe-only` output:

```sh
lvmsync run --probe-only /dev/vg0/snap0 /dev/vg0/target
# 10737418240 12345678-9abc-def0-1234-56789abcdef0 1700000000
```

## Device identity enforcement

Each transfer records `(device_id, fs_uuid, size_bytes, major:minor, manifest_epoch)` and compares it with the destination and any resume state before writing. LVMSync refuses to resume when this tuple differs, preventing accidental or malicious overwrites.

## Required sudoers entries

Tight `sudoers` rules limit what the helper may execute. LVMSync assumes the
controlling user can invoke **only** the following commands via `sudo`; any
broadly scoped or wildcard entries break the least-privilege model and may give
the user full root access. See [docs/sudoers.md](docs/sudoers.md) for
command-specific guidance:

```sudoers
# Allow LVM administration commands
lvmsync ALL=(root) NOPASSWD: /sbin/lvm, /sbin/lvcreate, /sbin/lvremove, /sbin/lvs, /sbin/pvs, /sbin/vgs

# Permit opening devices and issuing writes or discards through the helper
lvmsync ALL=(root) NOPASSWD: \\
    /usr/local/bin/lvmsync-helper open, \\
    /usr/local/bin/lvmsync-helper write, \\
    /usr/local/bin/lvmsync-helper discard

# Enable direct blkdiscard when the helper is unavailable
lvmsync ALL=(root) NOPASSWD: /usr/sbin/blkdiscard
```

Adjust paths to match your distribution.

## Least-privilege assumptions and risks

The project assumes a dedicated `lvmsync` account with the above narrowly
scoped rules. Granting broader access—such as allowing `lvmsync-helper` with
arbitrary arguments or permitting `lvm` without explicit subcommands—lets an
attacker escalate to full root control. Because the helper can read and write
raw block devices, a compromised account or misconfigured rule could lead to
complete destruction or disclosure of data across the entire disk.

## Environment sanitization

The helper normally inherits the caller's environment, including `PATH` and
`LANG`, as well as potentially unsafe variables such as `LD_PRELOAD` or
`GCONV_PATH`. Pass the `--sanitize-env` flag or enable the `SanitizeEnv` option
to run the helper with a minimal, whitelisted environment that drops those
variables entirely. Sanitization is disabled by default to avoid surprising
behavior in mixed environments.

## Logging hygiene

Operational logs should never include secret material. Avoid recording
private keys, certificates, passphrases, or similar credentials. The logger's
redaction hook can strip sensitive fields before they are written, and new log
statements should be reviewed for potential disclosure risks.

## Risks of raw-device writes

Granting raw-device access lets the helper overwrite any block on the target.
A misconfigured `sudoers` rule or path could destroy unrelated data or allow
full-disk compromise. LVMSync mitigates this risk by:

* Requiring explicit device paths; globbing and symlinks are rejected.
* Verifying device size and LVM signatures before writing.
* Dropping privileges immediately after completing the privileged section.

Review `sudoers` entries carefully and test on non-production systems before granting wide access.
