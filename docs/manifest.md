# Manifest Format

LVMSync writes a binary manifest alongside each transfer. The manifest tracks
chunk offsets and digests so that interrupted sessions can resume and completed
copies can be verified.

## Layout and Versioning

The file begins with a 120 byte little‑endian header:

| Field       | Size | Description |
|-------------|-----:|-------------|
| `version`   | 4    | Manifest format version (`1` today) |
| `block_size`| 4    | Device block size in bytes |
| `size_bytes`| 8    | Total device size |
| `chunk_count` | 8 | Number of chunks tracked |
| `device_id` | 64   | Persistent device identifier |
| `mac`       | 32   | BLAKE3 digest of the preceding header fields |

Each subsequent entry describes one chunk and also uses little‑endian encoding:

| Field     | Size | Description |
|-----------|-----:|-------------|
| `offset`  | 8    | Byte offset of the chunk |
| `length`  | 4    | Chunk length in bytes |
| _pad_     | 4    | Reserved for future use |
| `xxh3`    | 8    | Fast non‑cryptographic hash |
| `blake3`  | 32   | BLAKE3 digest of the chunk |

The header `version` field allows future format changes without breaking
backwards compatibility.

Device identifiers are stored in a fixed 64-byte field; creation fails if the
ID exceeds this limit.

## Resume Tokens

During a transfer handshake the sender may advertise a `resume:<token>` token.
The token encodes the header MAC and the last fully transferred chunk. This
prevents resuming against the wrong device and permits the receiver to skip
already replicated chunks.

## Rebuilding

Generate a manifest for an existing device when the index is missing or stale:

```sh
lvmsync manifest rebuild /dev/vg0/lv0
```
Progress logs are emitted every 10s by default; adjust with `--manifest-progress-interval`.
The command aborts if the device is mounted read-write. Override with `--manifest-allow-mounted` when rebuilding a live filesystem.

## Verification

Use a manifest to verify that a source and destination match:

```sh
lvmsync verify --manifest_path snapshot.manifest /dev/vg0/snap0 /dev/null
```

### Flags

| Flag | Environment variable | Config key | Description |
|------|----------------------|------------|-------------|
| `--resume` | `LVMSYNC_RESUME` | `resume` | Path to resume state file |
| `--verify_checksum` | `LVMSYNC_VERIFY_CHECKSUM` | `verify_checksum` | Enable checksum verification |

## Raw Device Safety

Working on a live block device can lead to inconsistent manifests if writes
occur during the scan. To ensure a stable view:

- `--offline` asserts that no process will modify the device while it is read.
- `--fs-freeze-command`/`--fs-thaw-command` run commands that freeze the
  filesystem and thaw it once the read completes.

Example scripts in this repository:

- [fsfreeze-freeze.sh](fsfreeze-freeze.sh)
- [fsfreeze-thaw.sh](fsfreeze-thaw.sh)

Use them together:

```sh
lvmsync manifest rebuild --fs-freeze-command "./docs/fsfreeze-freeze.sh /mnt" --fs-thaw-command "./docs/fsfreeze-thaw.sh /mnt" /dev/sdb
```
