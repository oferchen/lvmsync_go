# Security Model

LVMSync splits privileged and unprivileged responsibilities. The main controller runs as a regular user and delegates any operation that requires elevated permissions to a small, audited helper.

## Non-root controller and helper

The controller orchestrates replication, validates parameters, and interacts with remote peers. Operations that require root — such as managing LVM volumes, opening raw block devices or issuing discard commands — are executed through a privileged helper invoked via `sudo` or Linux capabilities. The helper performs only the requested action and exits immediately, minimising the trusted code surface.

## Example sudoers configuration

Tight `sudoers` rules limit what the helper may execute:

```sudoers
# Allow LVM administration commands
lvmsync ALL=(root) NOPASSWD: /sbin/lvm, /sbin/lvcreate, /sbin/lvremove, /sbin/lvs, /sbin/pvs, /sbin/vgs

# Permit opening block devices through the helper
lvmsync ALL=(root) NOPASSWD: /usr/local/bin/lvmsync-helper open, /usr/local/bin/lvmsync-helper write

# Enable discarding unused blocks
lvmsync ALL=(root) NOPASSWD: /usr/sbin/blkdiscard
```

Adjust paths to match your distribution.

## Risks of raw-device writes

Granting raw-device access lets the helper overwrite any block on the target. A misconfigured path or bug could destroy unrelated data. LVMSync mitigates this risk by:

* Requiring explicit device paths; globbing and symlinks are rejected.
* Verifying device size and LVM signatures before writing.
* Dropping privileges immediately after completing the privileged section.

Review `sudoers` entries carefully and test on non-production systems before granting wide access.
