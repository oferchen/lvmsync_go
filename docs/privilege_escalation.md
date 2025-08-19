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

By default LVMSync re-executes itself with the current `PATH` and `LANG`
values. Enable the `--sanitize-env` flag (or `LVMSYNC_SANITIZE_ENV=1`) to drop
both variables, providing a minimal environment to the helper process.



Roles are read from the certificate's `OrganizationalUnit` field and must
include `replicator` to invoke replication RPCs such as `StartSync`, `Cancel`,
`ProgressStream`, `BuildManifest`, and `Verify`. Any user-supplied metadata is
ignored, and requests without a verified role are rejected.
