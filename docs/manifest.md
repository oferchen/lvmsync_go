# Manifest Format

LVMSync tracks chunk metadata in an mmap-backed binary manifest. Each entry records the chunk
offset, length, XXH3 hash, and BLAKE3 digest. The manifest allows transfers to skip unchanged
blocks and verify destinations without storing duplicate metadata in memory.

Entries are laid out in fixed-width binary form for efficient random access. The header contains
device information, size, block size, and a BLAKE3 MAC over the metadata.

## Rebuilding

Regenerate a manifest for an existing device when the index is missing or out of date:

```sh
lvmsync manifest rebuild /dev/vg0/lv0
```

## Verification

Use manifests to verify that a source matches the recorded digests:

```sh
lvmsync verify --manifest snapshot.manifest /dev/vg0/snap0 /dev/null
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
