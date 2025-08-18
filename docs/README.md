# Documentation Index

- [Compression](compression.md) – chunk classifier, thresholds, and algorithm tuning.
- [Manifest Format](manifest.md) – rebuild and verification workflow.
- [Transports](transports.md) – transport negotiation and options.
- [LVM Snapshots](lvm.md) – snapshot creation, extension, discard, and mount checks.
- [Verification](verify.md) – command usage for integrity checks.
- [Privilege Escalation](privilege_escalation.md) – configuring LVM privilege escalation.
- [Daemon](daemon.md) – module configuration, ACLs, and listener options.
- Configuration precedence – flags override environment variables, which in
  turn override `config.yaml`. Unknown YAML keys emit warnings and invalid
  overrides are rejected.
