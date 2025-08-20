# Operations

This guide outlines exit codes, resume workflows, and common troubleshooting steps for LVMSync.

## Strict configuration

`--strict-config` or `LVMSYNC_STRICT_CONFIG=1` converts configuration warnings into errors, allowing CI pipelines to fail fast when unknown or deprecated settings are present.

## Resume and Verification Sequences

### `--probe-only`

```sh
lvmsync run --probe-only /dev/vg0/snap0 /dev/vg0/target
# 10737418240 12345678-9abc-def0-1234-56789abcdef0 9abcdef0-1234-5678-90ab-cdef12345678 0fedcba9-8765-4321-0fed-cba987654321 253 0 1700000000
```

Validates device identities and privileges and prints `size_bytes kernel_uuid gpt_uuid mbr_signature fs_uuid major minor manifest_epoch` without writing data.

### `--resume`

```sh
lvmsync run --resume=statefile /dev/vg0/snap0 /dev/vg0/target
```

1. Load the resume state and validate settings.
2. Validate previously written blocks against the manifest before applying new data unless `--verify=none`.
3. Copy remaining blocks, persisting checkpoints.
4. Delete the state file when the transfer finishes.
5. If the command fails, rerun with the same `--resume` file after fixing the issue.

### `--resume=verify`

```sh
lvmsync run --resume=verify /dev/vg0/snap0 /dev/vg0/target
```

Reloads the write-ahead log and rechecks previously written ranges against the manifest
before copying remaining data. This detects corrupted or tampered WAL entries before
continuing.

### `--verify-only`

```sh
lvmsync run --verify-only /dev/vg0/snap0 /dev/vg0/target
```

Reads both devices and reports mismatches without writing data.

## Skipping Snapshot Creation

`--skip-snapshot-creation` uses the source volume directly without creating a snapshot. To confirm the operator understands the risk, this flag must be combined with `--force`:

```sh
lvmsync run --skip-snapshot-creation --force /dev/vg0/src /dev/vg0/target
```

Snapshots created by LVMSync are tracked globally and removed if the process
receives `SIGINT`, `SIGTERM`, or panics.

## Automatic Destination Creation

`--create-dest-lv` creates the destination logical volume when it does not
exist. This is useful for fresh replication targets:

```sh
lvmsync run --create-dest-lv /dev/vg0/src /dev/vg0/dest
```

## Safe Overwrite Procedure

1. Probe devices and ensure privileges are correct:

   ```sh
   lvmsync run --probe-only /dev/vg0/snap0 /dev/vg0/target
   # 10737418240 12345678-9abc-def0-1234-56789abcdef0 9abcdef0-1234-5678-90ab-cdef12345678 0fedcba9-8765-4321-0fed-cba987654321 253 0 1700000000
   ```

2. Verify existing blocks:

   ```sh
   lvmsync run --verify-only /dev/vg0/snap0 /dev/vg0/target
   ```

3. Perform the copy, optionally resuming with verification:

   ```sh
   lvmsync run --resume=verify /dev/vg0/snap0 /dev/vg0/target
   ```

Exit code `60` indicates a verification mismatch and leaves the destination untouched.

### Safe Overwrite Test

The integration test `integration/safe_overwrite.sh` exercises this sequence:

1. `lvmsync run --probe-only /dev/vg0/snap0 /dev/vg0/target` exits `0` without writing.
2. `lvmsync run --verify-only /dev/vg0/snap0 /dev/vg0/target` exits `60` when blocks differ and leaves data unchanged.
3. `lvmsync run /dev/vg0/snap0 /dev/vg0/target` performs the actual copy.

The test confirms the probe and verify steps do not modify the destination logical volume.

## WAL Crash Safety

WAL updates are written to a temporary file and `fsync`ed before atomically
renaming to the final path. The parent directory is then `fsync`ed to persist the
rename. After a crash LVMSync validates the WAL header and replays only fully
committed ranges.

## Exit Codes and Recovery

| Constant | Code | Meaning | Recovery Step |
|----------|------|---------|---------------|
| [`exitcode.OK`](internal/exitcode/exitcode.go) | `0`  | Success | None |
| [`exitcode.ErrCapability`](internal/exitcode/exitcode.go) | `10` | Privilege or capability check failed | Run as root or adjust `--lvm-escalation`. |
| [`exitcode.ErrDevice`](internal/exitcode/exitcode.go) | `20` | Device error | Verify device paths and snapshot health. |
| [`exitcode.ErrSnapshotExhausted`](internal/exitcode/exitcode.go) | `25` | Snapshot space exhausted | Grow or recreate the snapshot before resuming. |
| [`exitcode.ErrPlatform`](internal/exitcode/exitcode.go) | `30` | Unsupported platform | Run on a supported Linux platform. |
| [`exitcode.ErrConfig`](internal/exitcode/exitcode.go) | `40` | Configuration error | Review flags, environment variables, and `config.yaml`. |
| [`exitcode.ErrRuntime`](internal/exitcode/exitcode.go) | `50` | Runtime failure | Inspect logs, fix the issue, and rerun using `--resume` when applicable. |
| [`exitcode.ErrVerify`](internal/exitcode/exitcode.go) | `60` | Verification mismatch | Investigate mismatched data before retrying. |
| [`exitcode.ErrPartial`](internal/exitcode/exitcode.go) | `70` | Partial transfer | Address the error and resume with `--resume`. |
| [`exitcode.ErrPrecondition`](internal/exitcode/exitcode.go) | `80` | Precondition failed | Fix prerequisites and retry. |
| [`exitcode.ErrResumable`](internal/exitcode/exitcode.go) | `90` | Resumable exit | Resume with `--resume` after resolving the issue. |

Definitions live in [internal/exitcode](internal/exitcode/exitcode.go).

Verification mismatches exit with [`exitcode.ErrVerify`](internal/exitcode/exitcode.go):

```sh
lvmsync run --verify-only /dev/vg0/snap0 /dev/vg0/bad_target || echo "verify failed with exit $?"
# verify failed with exit 60
```

Precondition failures exit with [`exitcode.ErrPrecondition`](internal/exitcode/exitcode.go):

```sh
lvmsync run /dev/vg0/missing /dev/vg0/target || echo "precondition failed with exit $?"
# precondition failed with exit 80
```

Partition-table changes between runs also trigger this error when GPT or MBR signatures differ.

## Troubleshooting

- Compare source and destination identities; the device identity tuple `(size_bytes, kernel_uuid, gpt_uuid, mbr_signature, fs_uuid, major, minor, manifest_epoch)` must match the resume file. LVMSync refuses to resume when the tuple differs.
- Confirm the destination is not mounted read-write. Use `--force` only when intentionally overwriting.
- Rerun with `--resume` after resolving issues to avoid re-copying completed blocks.
- Review logs for detailed errors and ensure all configuration values follow the expected flag > environment variable > config file precedence.

### Snapshot Overflow

Snapshot volumes fill when copy-on-write blocks exceed the allocated snapshot size.
LVMSync exits with [`exitcode.ErrDevice`](internal/exitcode/exitcode.go) (`20`).
Grow the snapshot or create a larger one, then rerun with the same `--resume` state:

```sh
lvmsync run /dev/vg0/snap_full /dev/vg0/dst || echo "snapshot overflow exit $?"
# snapshot overflow exit 20
```

### Verify Failure

Verification mismatches stop the transfer with
[`exitcode.ErrVerify`](internal/exitcode/exitcode.go) (`60`). Inspect the logs to
identify mismatched blocks before retrying:

```sh
lvmsync run --verify-only /dev/vg0/snap0 /dev/vg0/target || echo "verify exit $?"
# verify exit 60
```

### Resume After Interruption

Unexpected interruptions (signals, network loss) exit with
[`exitcode.ErrResumable`](internal/exitcode/exitcode.go) (`90`). Fix the underlying
issue and resume the transfer:

```sh
lvmsync run --resume state /dev/vg0/snap0 /dev/vg0/target
```

### Identity Tuple Mismatch

If the source or destination no longer matches the resume state, LVMSync exits with
[`exitcode.ErrPrecondition`](internal/exitcode/exitcode.go) (`80`) and refuses to resume. Recreate the
destination or regenerate the resume state before restarting.

## Failure Drills

Practice recovery steps regularly so operators are prepared for common failure
scenarios.

### Snapshot Full

1. Create a small snapshot and start a transfer:
   ```sh
   lvcreate -L1G -s -n snap_full /dev/vg0/src
   lvmsync run /dev/vg0/snap_full /dev/vg0/dst &
   ```
2. Write more than 1 GiB to the source to exhaust the snapshot:
   ```sh
   dd if=/dev/zero of=/dev/vg0/src bs=1M count=2048
   ```
3. LVMSync exits with a device error. Remove the snapshot, create a larger one,
   and resume with the same command line.

### Identity Mismatch

1. Begin a transfer and capture the resume state file.
2. Modify the destination (e.g., recreate the LV) and rerun with `--resume`:
   ```sh
   lvremove -y /dev/vg0/dst
   lvcreate -L10G -n dst /dev/vg0
   lvmsync run --resume state /dev/vg0/snap0 /dev/vg0/dst || echo "mismatch"
   ```
3. LVMSync reports a precondition failure due to the changed identity. Fix the
   destination and restart without `--resume` or regenerate the state.

### TLS Failure

1. Run a remote transfer with an invalid certificate or hostname:
   ```sh
   lvmsync run --remote https://badhost /dev/vg0/snap0 user@host:/dev/vg0/dst
   ```
2. The TLS handshake fails with `exitcode.ErrRuntime`. Verify certificates and
   trust stores, then retry the transfer.

