# Manifest Format

LVMSync records transfer metadata in a manifest file. Each entry stores the chunk offset, length, and BLAKE3 digest so transfers can resume and destinations can be verified.

## Index Format

Manifests are JSON lines with one object per chunk:

```json
{"offset":0,"size_bytes":4096,"digest":"<hex blake3>"}
```

The final line carries a SHA-256 digest of all chunk digests to validate completeness.

## Rebuilding

Regenerate a manifest for an existing device when the index is missing or out of date:

```sh
lvmsync manifest rebuild /dev/vg0/lv0
```

## Verification

Use manifests to verify that a source and destination match:

```sh
lvmsync verify /dev/vg0/snap0 /mnt/backup
```

### Flags

| Flag | Environment variable | Config key | Description |
|------|----------------------|------------|-------------|
| `--resume` | `LVMSYNC_RESUME` | `resume` | Path to resume state file |
| `--verify_checksum` | `LVMSYNC_VERIFY_CHECKSUM` | `verify_checksum` | Enable checksum verification |

## Raw Device Safety

Working on a live block device can lead to inconsistent manifests if writes occur during the scan. To ensure a stable view:

- `--offline` asserts that no process will modify the device while it is read.
- `--fs-freeze-command` runs a command that freezes the filesystem and thaws it once the read completes.

Example scripts in this repository:

- [fsfreeze-freeze.sh](fsfreeze-freeze.sh)
- [fsfreeze-thaw.sh](fsfreeze-thaw.sh)

Use them together:

```sh
lvmsync manifest rebuild --fs-freeze-command "./docs/fsfreeze-freeze.sh /mnt && ./docs/fsfreeze-thaw.sh /mnt" /dev/sdb
```
