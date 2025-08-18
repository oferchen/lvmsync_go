# Operations

This guide outlines exit codes, resume workflows, and common troubleshooting steps for LVMSync.

## Resume and Verification Sequences

### `--resume`

```sh
lvmsync run --resume statefile /dev/vg0/snap0 /dev/vg0/target
```

1. Load the resume state and validate settings.
2. Copy remaining blocks, persisting checkpoints.
3. Delete the state file when the transfer finishes.
4. If the command fails, rerun with the same `--resume` file after fixing the issue.

### `--resume=verify`

```sh
lvmsync run --resume=verify /dev/vg0/snap0 /dev/vg0/target
```

1. Resume the transfer using the existing state.
2. After the copy completes, `lvmsync verify` checks the destination against the source or manifest.
3. A verification failure keeps the resume file for another attempt.

### `--verify-only`

```sh
lvmsync run --verify-only /dev/vg0/snap0 /dev/vg0/target
```

Reads both devices and reports mismatches without writing data.

## Exit Codes and Recovery

| Code | Meaning | Recovery Step |
|------|---------|---------------|
| `0`  | Success | None |
| `10` | Privilege or capability check failed | Run as root or adjust `--lvm-escalation`. |
| `20` | Device error | Verify device paths and snapshot health. |
| `30` | Unsupported platform | Run on a supported Linux platform. |
| `40` | Configuration error | Review flags, environment variables, and `config.yaml`. |
| `50` | Runtime failure | Inspect logs, fix the issue, and rerun using `--resume` when applicable. |
| `60` | Verification mismatch | Investigate mismatched data before retrying. |
| `70` | Partial transfer | Address the error and resume with `--resume`. |

## Troubleshooting

- Compare source and destination identities; the device identity tuple `(device_id, size_bytes, major:minor)` must match the resume file.
- Confirm the destination is not mounted read-write. Use `--force` only when intentionally overwriting.
- Rerun with `--resume` after resolving issues to avoid re-copying completed blocks.
- Review logs for detailed errors and ensure all configuration values follow the expected flag > environment variable > config file precedence.

