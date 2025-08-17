# Manifest Format

LVMSync writes a binary manifest alongside each transfer. The manifest tracks
chunk offsets and digests so that interrupted sessions can resume and completed
copies can be verified.

## Lifecycle

1. Rebuild a manifest for the source device:
   `lvmsync manifest rebuild <device>`
2. Run a transfer using `lvmsync run`.
3. Verify that the destination matches the manifest:
   `lvmsync verify <source> <dest>`

## Usage

- `lvmsync manifest rebuild <device>` regenerates a manifest when one is
  missing or out of date.
- `lvmsync verify <source> <dest>` compares a destination against the manifest
  for the source. Override the manifest path with `--manifest_path` if needed.

Manifest commands group flags with pflag and bind them to Viper while logging progress with zap.
Flags override environment variables, which override `config.yaml` values.

| Flag | Environment variable | Config key | Description |
|------|----------------------|------------|-------------|
| `--manifest_path` | `LVMSYNC_MANIFEST_PATH` | `manifest_path` | Path to manifest file |
| `--manifest_timeout` | `LVMSYNC_MANIFEST_TIMEOUT` | `manifest_timeout` | Timeout for manifest rebuild (0 disables) |
| `--manifest_progress_interval` | `LVMSYNC_MANIFEST_PROGRESS_INTERVAL` | `manifest_progress_interval` | Interval between progress logs during manifest rebuild |
| `--manifest_allow_mounted` | `LVMSYNC_MANIFEST_ALLOW_MOUNTED` | `manifest_allow_mounted` | Allow rebuilding when device is mounted read-write |

## Flag Group Example

```go
import (
    "github.com/spf13/pflag"
    "github.com/spf13/viper"
    "go.uber.org/zap"
)

func initFlags() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    fs := pflag.NewFlagSet("manifest", pflag.ExitOnError)
    fs.String("manifest-path", "", "path to manifest file")

    v := viper.New()
    v.BindPFlags(fs)
}
```

Chunk offsets are determined using FastCDC. The gear table now uses the
standard 256-entry random values from the FastCDC specification, replacing the
placeholder table previously used.

## Security Defaults

- The header MAC binds version, block size, device size, chunk count, and
  device ID to detect tampering.
- Rebuild aborts if the device is mounted read-write; override with
  `--manifest-allow-mounted` only when the filesystem is quiesced.
- Resume tokens encode the header MAC and last chunk to prevent resuming on a
  different device.
- Raw device scans require `--offline` or explicit freeze/thaw hooks to keep
  data consistent.

## Layout and Versioning

The file begins with a 136 byte little‑endian header:

| Field          | Size | Description |
|----------------|-----:|-------------|
| `version`      | 4    | Manifest format version (`2` today) |
| `block_size`   | 4    | Device block size in bytes |
| `size_bytes`   | 8    | Total device size |
| `chunk_count`  | 8    | Number of chunks tracked |
| `cdc_min`      | 4    | Minimum CDC chunk size |
| `cdc_avg`      | 4    | Average CDC chunk size |
| `cdc_max`      | 4    | Maximum CDC chunk size |
| `hybrid_fixed` | 4    | Fixed chunk size when hybrid dedup is used (0 otherwise) |
| `device_id`    | 64   | Persistent device identifier |
| `mac`          | 32   | BLAKE3 digest of the preceding header fields |

Each subsequent entry describes one chunk and also uses little‑endian encoding:

| Field     | Size | Description |
|-----------|-----:|-------------|
| `offset`  | 8    | Byte offset of the chunk |
| `length`  | 4    | Chunk length in bytes |
| `flags`   | 4    | Chunk metadata flags (`1` marks CDC chunks) |
| `xxh3`    | 8    | Fast non‑cryptographic hash |
| `blake3`  | 32   | BLAKE3 digest of the chunk |

The header `version` field allows future format changes without breaking
backwards compatibility.

## Two-Level Index

When a manifest is opened, LVMSync builds an in-memory two-level index to
accelerate lookups. A primary open-addressed hash table maps each chunk's XXH3
value to its entry index. To avoid unnecessary BLAKE3 computations, every
1&nbsp;MiB region also maintains a 64‑bit Bloom filter of XXH3 hashes present in
that range. During `Match`, the Bloom filter for the chunk's range is checked
first; only if it may contain the hash does the hash table probe proceed and, if
needed, the strong digest is computed. `Rebuild` streams the device and
populates both the hash table and Bloom filters on the fly.

Device identifiers are stored in a fixed 64-byte field; creation fails if the
ID exceeds this limit.

## Resume Tokens

During a transfer handshake the sender may advertise a `resume:<token>` token.
The token encodes the header MAC and the last fully transferred chunk. This
prevents resuming against the wrong device and permits the receiver to skip
already replicated chunks.

## Examples

### Rebuild

Generate a manifest for an existing device when the index is missing or stale:

```sh
lvmsync manifest rebuild /dev/vg0/lv0
```

Progress logs are emitted every 10s by default; adjust with
`--manifest-progress-interval`. The rebuild operation times out after 1m unless
`--manifest_timeout` is set (0 disables).

### Verify

Use a manifest to verify that a source and destination match:

```sh
lvmsync verify /dev/vg0/snap0 /dev/null
```

#### Flags

| Flag | Environment variable | Config key | Description |
|------|----------------------|------------|-------------|
| `--resume` | `LVMSYNC_RESUME` | `resume` | Path to resume state file |
| `--verify_checksum` | `LVMSYNC_VERIFY_CHECKSUM` | `verify_checksum` | Enable checksum verification |
| `--digest` | `LVMSYNC_DIGEST` | `digest` | Digest algorithm: `auto`, `blake3`, or `sha256` |

### Freeze and Thaw Live Filesystems

Working on a live block device can lead to inconsistent manifests if writes
occur during the scan. To ensure a stable view:

- `--offline` asserts that no process will modify the device while it is read.
- `--fs-freeze-command`/`--fs-thaw-command` run commands that freeze the
  filesystem and thaw it once the read completes. Command paths must be
  absolute.

Example scripts in this repository:

- [fsfreeze-freeze.sh](fsfreeze-freeze.sh)
- [fsfreeze-thaw.sh](fsfreeze-thaw.sh)

Use them together:

```sh
lvmsync manifest rebuild --fs-freeze-command "$(pwd)/docs/fsfreeze-freeze.sh /mnt" --fs-thaw-command "$(pwd)/docs/fsfreeze-thaw.sh /mnt" /dev/sdb
```
