## `sudoers` Entries

LVMSync requires elevated privileges for a small set of operations. The
following rules use `Cmnd_Alias` to keep entries concise while limiting access
to only the necessary commands. Adjust paths to match your distribution and
test on non-production systems before enabling them in production.

```sudoers
Cmnd_Alias LVMSYNC_LVM = \
    /sbin/lvm, \
    /sbin/lvcreate, \
    /sbin/lvremove, \
    /sbin/lvs, \
    /sbin/pvs, \
    /sbin/vgs

Cmnd_Alias LVMSYNC_HELPER = \
    /usr/local/bin/lvmsync-helper open, \
    /usr/local/bin/lvmsync-helper write, \
    /usr/local/bin/lvmsync-helper discard

Cmnd_Alias LVMSYNC_BLKDISCARD = /usr/sbin/blkdiscard

# Allow the "lvmsync" user to run the aliases without a password
lvmsync ALL=(root) NOPASSWD: LVMSYNC_LVM, LVMSYNC_HELPER, LVMSYNC_BLKDISCARD
```

Test each entry carefully to ensure no additional privileges are granted.
