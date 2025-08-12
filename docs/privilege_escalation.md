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
