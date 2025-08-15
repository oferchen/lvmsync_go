# Privilege Escalation Setup

LVMSync operates on block devices and prefers Linux capabilities over sudo.

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

## gRPC Role Mapping

The gRPC control plane derives authorization roles from client TLS certificates.
Roles are read from the certificate's `OrganizationalUnit` field and must
include `replicator` to invoke replication RPCs such as `StartSync`, `Cancel`,
`ProgressStream`, `BuildManifest`, and `Verify`. Any user-supplied metadata is
ignored, and requests without a verified role are rejected.
