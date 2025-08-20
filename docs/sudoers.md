## `sudoers` Entries

LVMSync requires elevated privileges for a small set of operations. The
following examples show least-privilege `sudoers` rules for each command.
See [../SECURITY.md](../SECURITY.md) for the overall model, assumptions, and
risks. Adjust paths to match your distribution and test on non-production
systems before enabling them in production.

All command paths are validated by LVMSync at startup. Escalation commands
must be specified explicitly and may not contain pipes or shell redirects.

## LVM administration

Permit only the LVM utilities needed by LVMSync:
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

# Allow the "lvmsync" user to run the aliases without a password and block
# shell escapes and environment injection
lvmsync ALL=(root) NOPASSWD:NOEXEC,SETENV=!setenv: LVMSYNC_LVM, LVMSYNC_HELPER, LVMSYNC_BLKDISCARD
# Older sudo versions may require the following syntax instead:
# lvmsync ALL=(root) NOPASSWD:NOEXEC,!setenv: LVMSYNC_LVM, LVMSYNC_HELPER, LVMSYNC_BLKDISCARD
```

`NOEXEC` prevents privileged commands from launching other binaries through
shell escapes, while `!setenv` blocks attackers from altering the execution
environment. These restrictions help contain compromised helpers and reduce
privilege-escalation risks.

Test each entry carefully to ensure no additional privileges are granted.

## Auditing

Monitor the journal or syslog to verify only expected commands run with elevated privileges:

```sh
# systemd journal examples
journalctl _COMM=sudo --since -1h
journalctl -t lvmsync-helper

# syslog-based systems
grep lvmsync-helper /var/log/auth.log
```

Review these logs regularly to confirm the helper and LVM commands execute as intended.
