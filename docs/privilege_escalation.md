# Privilege Escalation Setup

LVMSync operates on block devices and prefers Linux capabilities over sudo.

For security design and sudoers examples, see [SECURITY.md](../SECURITY.md).

## Capabilities

Grant required capabilities to the binary:

```sh
setcap cap_sys_admin,cap_sys_rawio,cap_dac_override+ep /usr/local/bin/lvmsync
```

## sudoers Fallback

When capabilities are unavailable, provide a minimal sudoers entry:

```
lvmsync ALL=(root) NOPASSWD:/usr/local/bin/lvmsync
```

Verify escalation availability:

```sh
lvmsync --check-escalation
```

## Environment Sanitization

LVMSync forwards the invoking environment unchanged. Use `--sanitize-env` or
`LVMSYNC_SANITIZE_ENV=1` to drop `PATH`, `LANG`, and unsafe variables like
`LD_PRELOAD` before invoking `sudo`. The helper also clears any ambient
capability sets after each privileged command to limit the window of elevated
rights.

For additional hardening, pass `--no-new-privs` or set `LVMSYNC_NO_NEW_PRIVS=1` to call `prctl(PR_SET_NO_NEW_PRIVS)` before `sudo` execution. This prevents escalated commands from gaining further privileges.



Roles are read from the certificate's `OrganizationalUnit` field and must
include `replicator` to invoke replication RPCs such as `StartSync`, `Cancel`,
`ProgressStream`, `BuildManifest`, and `Verify`. Any user-supplied metadata is
ignored, and requests without a verified role are rejected.
