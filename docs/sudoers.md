# sudoers Entries

LVMSync requires elevated privileges for a small set of operations. The
following examples show least-privilege `sudoers` rules for each command.
Adjust paths to match your distribution and test on non-production systems
before enabling them in production.

## LVM administration

Permit only the LVM utilities needed by LVMSync:

```sudoers
# Allow LVMSync to run LVM commands without a password
lvmsync ALL=(root) NOPASSWD: \
    /sbin/lvm, \
    /sbin/lvcreate, \
    /sbin/lvremove, \
    /sbin/lvs, \
    /sbin/pvs, \
    /sbin/vgs
```

## Device helper

The helper performs privileged device operations such as opening block
devices, writing data and issuing discards:

```sudoers
lvmsync ALL=(root) NOPASSWD: \
    /usr/local/bin/lvmsync-helper open, \
    /usr/local/bin/lvmsync-helper write, \
    /usr/local/bin/lvmsync-helper discard
```

## blkdiscard fallback

Enable direct `blkdiscard` when the helper is unavailable:

```sudoers
lvmsync ALL=(root) NOPASSWD: /usr/sbin/blkdiscard
```

Test each entry carefully to ensure no additional privileges are granted.
