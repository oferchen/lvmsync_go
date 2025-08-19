# Operations

This guide covers snapshot lifecycle management, resume options, verification modes, and expected exit codes for recovery.

## STDOUT mode confirmation

`--stdout` streams raw binary data to standard output. When `lvmsync` detects
an interactive TTY on stdin, it prompts for confirmation before proceeding. In
non-interactive sessions the `--yes-i-know` flag (or `LVMSYNC_YES_I_KNOW`)
must be supplied to bypass the prompt.

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

### Crash Recovery

1. Start transfers with `--resume statefile` so checkpoints persist.
2. If the process is interrupted (for example, by `SIGKILL`), invoke the same
   command with `--resume statefile` to continue copying remaining blocks.
3. LVMSync validates the device identity tuple `(device_id, size_bytes,
   major:minor)` against the resume file. Mismatches abort with a precondition
   failure to prevent accidental overwrites.
4. After a successful resume the state file is removed; optionally run
   `lvmsync verify` to confirm both devices match.

### `--verify-only`

```sh
lvmsync run --verify-only /dev/vg0/snap0 /dev/vg0/target
```

Reads both devices and reports mismatches without writing data.

## WAL Crash Safety

The write-ahead log records completed block ranges so interrupted transfers can resume safely. Each appended range is fsynced before returning. On restart `OpenWAL` validates a MAC over the header and ensures the size, epoch, and device ID match the current transfer. Corrupted headers or tampered device IDs cause the log to be rejected, and entries written without a matching fsync are ignored after power loss.

## Exit Codes

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

