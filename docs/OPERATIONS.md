# Operations

This guide covers snapshot lifecycle management, resume options, verification modes, and expected exit codes for recovery.

## Snapshot Creation and Cleanup Flow

1. `lvmsync run` validates the source and destination.
2. A snapshot of the source logical volume is created unless `--skip-snapshot-creation` is used.
3. Snapshot usage is monitored while blocks are streamed to the destination.
4. On completion or interruption, the snapshot is removed.

## Resume and Verification Sequences

### `--resume`

```sh
lvmsync run --resume=statefile /dev/vg0/snap0 /dev/vg0/target
```

1. Load the resume state and validate settings.
2. Validate previously written blocks against the manifest before applying new data unless `--verify=none`.
3. Copy remaining blocks, persisting checkpoints.
4. Delete the state file when the transfer finishes.
5. If the command fails, rerun with the same `--resume` file after fixing the issue.

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
| `50` | Runtime failure | Inspect logs and fix the issue. |
| `60` | Verification mismatch | Investigate mismatched data before retrying. |
| `70` | Partial transfer | Address the error and resume with `--resume`. |

