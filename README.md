# LVMSync
[![Build Status](https://github.com/oferchen/lvmsync_go/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/oferchen/lvmsync_go/actions/workflows/go.yml)
[![Build Status](https://github.com/oferchen/lvmsync_go/actions/workflows/super-linter.yml/badge.svg?branch=main)](https://github.com/oferchen/lvmsync_go/actions/workflows/super-linter.yml)

LVMSync is a high-performance incremental data replication tool for LVM snapshots. It efficiently transfers only changed blocks using metadata from snapshot COW (Copy-On-Write) devices and communicates with LVM through native Go bindings rather than shell commands.

## Features

- **Incremental Block-Level Synchronization**: Transfers only changed blocks.
- **Zero-Copy Transfers**: Utilizes `splice()` for efficient data movement.
- **Parallel Execution**: Configurable concurrency for optimal performance.
- **Adaptive Transport Concurrency**: Maintains ~1–2×BDP of in-flight data and can be overridden with `--concurrency`.
- **Rate-Limiting**: Control bandwidth usage during transfers.
- **Compression**: Samples 8 KiB per chunk, skipping compression when the ratio exceeds a threshold. Auto mode selects LZ4 for chunks <256 KiB and Zstd level 1 for larger chunks on CPUs with AVX2 or NEON support.
- **Checksum Verification**: Ensures data integrity using SHA-256 or BLAKE3, automatically selecting BLAKE3 on CPUs with AES-NI, AVX2/AVX-512, or NEON.
- **Native LVM2 Integration**: Uses Go bindings to `liblvm2cmd` instead of shelling out.
- **Generic Block Device Support**: Access raw `/dev/*` paths and regular files (including loopback images) through a unified device abstraction.
- **Deduplication Strategies**: Detect unchanged blocks using checksum, rolling hash, or a Bloom filter with optional FastCDC content-defined chunking and mmap-backed index.
- **Hashing**: Hardware-accelerated XXH3 provides fast deduplication hints while BLAKE3 digests are stored in manifests for integrity.
- **Remote Execution via SSH**: Replicates data over SSH with support for pre/post-scripts.
- **Resume Support**: Ability to resume interrupted transfers.
- **Handshake Timeouts**: Transport connections apply context deadlines during handshakes and clear them once negotiation succeeds.
- **Sparse Destination Optimization**: Detects runs of zero bytes and punches holes when the filesystem supports it.
- **Aligned I/O Buffers and NUMA Pinning**: `--odirect` allocates block-size aligned slabs from a `sync.Pool` and can pin worker goroutines to the device's NUMA node.
- **LVM Snapshot Management**:
  - Automatic snapshot creation and removal.
  - Configurable snapshot size (absolute or percentage-based) via `--snapshot_size`,
    `LVMSYNC_SNAPSHOT_SIZE`, or the `snapshot_size` config key.
  - Configurable volume group for constructing the snapshot device path.
  - Auto-selection of target volume groups with sufficient free space.
  - Automatic privilege escalation (defaulting to `sudo -n`).
  - Snapshot health monitoring that fails fast if usage exceeds a threshold.
  - Snapshot monitor goroutine closes its error channel on exit; cleanup only
    cancels monitoring, avoiding send-on-closed-channel panics (see
    `TestCreateSnapshotCleanupNoPanic`).
- **Graceful Shutdown**: Signal handling ensures snapshots are cleaned up on interruption.
- **Flexible Configuration**: Flags, environment variables, or `config.yaml`. See [Configuration](#configuration).
- **Configuration Validation**: Checks key parameters (e.g., volume group existence, escalation command) before starting operations.

## Device Support Matrix

| Device type      | Source | Destination | Notes |
|------------------|:------:|:-----------:|-------|
| LVM snapshot     |   ✅   |      ❌      | snapshots are auto-created |
| Raw block device |   ✅   |      ✅      | requires `--offline` or `--fs-freeze-command`/`--fs-thaw-command` when used as a source |
| Regular file     |   ✅   |      ✅      | includes loopback images |

Override automatic detection with `--source-type` and `--dest-type` when a device's type is known in advance.

### Offline requirements

Raw sources must be quiescent or provide filesystem freeze/thaw hooks using
`--fs-freeze-command` and `--fs-thaw-command`. LVM snapshots are consistent by
design, while regular files require no additional coordination.

## Transport Options

LVMSync negotiates transports in the order provided by `--transport` (default
`quic,h2,tcp+tls,ssh`). All transports require TLS 1.3 with mutual
authentication unless `--allow-insecure` is set or the SSH transport is used.
See [docs/transports.md](docs/transports.md) for details.

| Transport | Security defaults | Notes |
|-----------|------------------|-------|
| `quic`    | TLS 1.3, BBR congestion | UDP-based transport |
| `h2`      | TLS 1.3                | HTTP/2 streams |
| `tcp+tls` | TLS 1.3                | Plain TCP wrapped in TLS |
| `ssh`     | Host key verification  | Uses OpenSSH-style authentication |

## Resume and Verify Workflows

Resume interrupted transfers with a state file:

```sh
lvmsync run --resume statefile /dev/vg0/snap0 /dev/vg0/data
```

Generate a manifest and verify a destination:

```sh
lvmsync manifest rebuild /dev/vg0/snap0
lvmsync verify /dev/vg0/snap0 /dev/vg0/data
```

Resume files track the last completed chunk and are removed after a successful
transfer. See [docs/manifest.md](docs/manifest.md) for manifest and verification details.

## Safety Notes

- Run `manifest rebuild` and `verify` against quiescent devices.
- Use `--offline` or freeze/thaw hooks when scanning live filesystems to keep
  manifests consistent.
- Network transports default to TLS 1.3; `--allow-insecure` should only be used
  for testing.

## Supported Platforms

LVMSync targets Linux systems only. Builds are tested on the `amd64` and `arm64` architectures.

## Roadmap

- gRPC control plane with mTLS and configurable ports
- Pluggable data plane: QUIC, HTTP/2, TLS/TCP, SSH
- Hybrid fixed + CDC deduplication with Bloom filter index
- Adaptive compression using LZ4 or Zstd with per-chunk sampling
- Throughput mode presets for high-bandwidth links

See AGENTS.md for contributor tasks and design guidelines.

## Architecture

LVMSync is organized into modular packages to keep concerns separated:

- `lvm` – manages snapshot creation, monitoring, and cleanup.
- `device` – opens and queries generic block devices such as raw `/dev/*` paths and regular files.
- `transfer` – performs block-level synchronization, compression, deduplication, and resume logic.
  - Internally split into focused modules: `progress.go`, `handshake.go`, and `block_writer.go` for clearer responsibilities.
- `remote` – wraps SSH functionality for running commands on remote hosts and coordinating transfers. Callers must provide a `context.Context` with a timeout when starting the privileged helper to allow cancellation if the remote command fails to launch.
- `config` – parses and validates configuration files and CLI options.
- `dedup` – houses Bloom filter helpers, chunking logic, and other deduplication utilities.
- `grpc` – provides the gRPC server and authentication helpers used by the remote daemon.
- `common` and `internal` – shared helpers and internal utilities such as multi-error handling.
- `internal/client` – coordinates snapshot preparation and client transfer execution.
- `cmd/dump` – handles snapshot dumping and transport selection.
- `cmd/root` – configures the application and routes to subcommands.
- `cmd/apply` – applies streamed data to destination devices.
- `cmd/lvmsync` – CLI orchestrator with a `signals` subpackage for signal handling and cleanup.
- `cmd/grpcd` – standalone gRPC daemon exposing LVMSync operations remotely.

This structure allows individual packages to be developed and tested in isolation.

### Refactoring Notes

- Snapshot preparation helpers (`ensureVolumeGroups`, `checkDiskSpaceForSnapshot`, `createSnapshotIfNeeded`, `PrepareSnapshot`) and client execution logic are consolidated under `internal/client`.
- These helpers no longer rely on global variables; configuration and loggers are passed explicitly.
- `main.go` now delegates to `cmd/root`, which wires together `cmd/dump` and `cmd/apply`.

## Logging

LVMSync emits structured logs using [zap](https://github.com/uber-go/zap). Errors are logged with
structured fields instead of being written to stderr, and the logger is flushed on shutdown to
ensure all entries are persisted. A production logger is initialized immediately so even
configuration failures during startup are reported through the same structured format. When
`--progress` is enabled, progress updates are emitted as structured log entries, allowing external
tooling to track transfer completion.

### Expectations

- Use `zap` for all logging and avoid `fmt.Print*` or `log.*` calls.
- Pass loggers explicitly to commands and helpers; `cmd/lvmsync.Execute` requires a
  `*zap.Logger` argument instead of relying on `zap.L()`.
- All commands receive an explicit `*zap.Logger` and default to `zap.NewNop()` when no logger is supplied.
- Device constructors return an error when the logger is `nil`; use `zap.NewNop()` to disable logging.
- Log field keys in `snake_case` and include units where relevant (for example, `duration_ms`).
- Provide raw byte values alongside human-readable sizes (for example, `block_size` and `block_size_bytes`).
- Always defer `syncLogger(logger)` to flush buffers and log if the sync fails.

The example below demonstrates these conventions:

```go
package main

import (
    "time"

    "go.uber.org/zap"
)

func syncLogger(logger *zap.Logger) {
    if err := logger.Sync(); err != nil {
        logger.Error("sync failed", zap.Error(err))
    }
}

func main() {
    logger, _ := zap.NewProduction()
    defer syncLogger(logger)
    start := time.Now()

    src := "/dev/vg0/source"
    dst := "/dev/vg0/backup"

    logger.Info("snapshot complete",
        zap.String("source_path", src),
        zap.String("dest_path", dst),
        zap.Int64("duration_ms", time.Since(start).Milliseconds()),
    )
}
```

Errors during block operations log the byte offset and block size explicitly:

```
Logger.Warn("Zero-copy transfer failed",
    zap.Int64("offset", offset),
    zap.Int("size_bytes", blockSize),
    zap.Int("attempt", attempt+1),
    zap.Error(err),
)
```

| Field | Description |
|-------|-------------|
| `offset` | Byte offset from the start of the device |
| `size_bytes` | Size of the block being processed |
| `attempt` | Current retry attempt |

## Configuration

LVMSync uses [`pflag`](https://github.com/spf13/pflag) and [`viper`](https://github.com/spf13/viper) to accept options from
flags, environment variables, and a YAML file. The CLI exposes subcommands using `cobra`, with `run` handling transfers,
`manifest rebuild` regenerating manifests, and `verify` checking source and destination data. Source and destination paths
for `run` and `verify` are provided as positional arguments after any flags:

```sh
lvmsync run [flags] <source> <dest>
```

When run with `--dry-run`, LVMSync loads any manifest at `--manifest_path` and samples up to 100 blocks to estimate the bytes that would be transmitted. The estimate and ETA in seconds (`eta_seconds`) are logged without sending data. For example:

```json
{"level":"info","msg":"dry run","size_bytes":4096,"estimated_tx_bytes":4096,"eta_seconds":2}
```

### Examples

Set the `parallel` worker count using any configuration source:

CLI flag:

```sh
lvmsync run --parallel 16
```

Environment variable:

```sh
LVMSYNC_PARALLEL=16 lvmsync run
```

`config.yaml`:

```yaml
parallel: 16
```

### Flag groups

Flags are grouped in the CLI help:

- **General Options** – worker counts, speed limits, progress controls.
- **SSH Options** – credentials and connection settings.
- **Remote Options** – remote hooks and lvmsync path.
- **Deduplication Options** – dedup strategy and state storage.
- **Compression Options** – algorithm and level tuning.
- **LVM Options** – snapshot management and privilege escalation.
- **gRPC Options** – parameters for the optional gRPC daemon.
- **Transport Options** – configure data transports (QUIC, HTTP/2, TCP+TLS, SSH).
- **Manifest Options** – manifest path overrides and related settings.

Internally, each group is set up through a dedicated helper such as
`initGeneralFlags`, `initSSHFlags`, or `initCompressionFlags`, keeping flag
definitions focused and easy to maintain.

Example:

```sh
lvmsync run /dev/vg0/snap0 /mnt/backup
```

```
func initConfig() *viper.Viper {
    v := viper.New()

    general := pflag.NewFlagSet("general", pflag.ExitOnError)
    general.Bool("progress", true, "show progress")

    lvm := pflag.NewFlagSet("lvm", pflag.ExitOnError)
    lvm.String("volume_group", "", "target volume group")

    pflag.CommandLine.AddFlagSet(general)
    pflag.CommandLine.AddFlagSet(lvm)

    v.BindPFlags(pflag.CommandLine)
    v.SetEnvPrefix("LVMSYNC")
    v.AutomaticEnv()
    v.SetConfigName("config")
    v.AddConfigPath(".")
    return v
}
```

### Grouped help

Each subcommand prints its relevant flag groups:

```
$ lvmsync run --help
General Options:
      --parallel int   number of worker goroutines (default 4)
...
Transport Options:
      --transport string   transport modes (comma-separated)

$ lvmsync manifest rebuild --help
General Options:
      --dry-run   skip execution
Manifest Options:
      --manifest_path string   manifest file path

$ lvmsync verify --help
General Options:
      --block_size string   block size for comparisons
Manifest Options:
      --manifest_path string   manifest to verify against

This groups related flags once and lets Viper merge values from flags, `LVMSYNC_*` variables, and the
`config.yaml` file.

The overall loading flow now passes an explicit `FlagSet` and argument slice:

1. `registerFlags(flagSets, fs)` adds all flag groups to the provided flag set.
2. `LoadConfig(flagSets, defaults, fs, args)` parses the arguments, binds flags and `LVMSYNC_*` environment variables with Viper,
   merges them with defaults, and returns the effective configuration along with any leftover positional arguments.

`cmd/root.Configure` surfaces those leftover arguments so `Run` operates purely on provided inputs.

### New and updated flags

Recent refactors added several configuration options:

- `--tcp_port` and `--ssh_port` expose TCP+TLS and SSH endpoints.
- `--tcp_parallel` controls the number of parallel TCP connections (2–4).
- `--tcp_lowat` sets TCP_NOTSENT_LOWAT to limit unsent bytes.
- `--sync-interval` controls how many bytes are written between `fdatasync` calls (flag uses underscores in the CLI: `--sync_interval`).
- `--checkpoint_interval` sets how often resume state is persisted.
- `--checkpoint_bytes` sets how many bytes are written between resume checkpoints.
- `--block_size` sets the transfer block size (use `auto` for detection).

### I/O tuning

- `--odirect` uses O_DIRECT with block-size aligned buffers.
- `--sync-interval` sets how many bytes are written between `fdatasync` calls (flag uses underscores in the CLI: `--sync_interval`).
- `--numa_pin` pins worker goroutines to CPUs local to the source device's NUMA node.

### Device types

LVMSync works with three kinds of source and destination devices. Auto-detection
examines the path to select the correct handling:

| Type | Detection | Notes |
|------|-----------|-------|
| `lvm` | `/dev/<vg>/<lv>` or `/dev/mapper/<vg>-<lv>` | A snapshot is created and removed automatically |
| `raw` | Other block devices | Require `--skip_snapshot_creation` and either `--offline` or `--fs-freeze-command`/`--fs-thaw-command` |
| `file` | Regular files | Used as-is with no snapshot |

Override detection with `--source-type` and `--dest-type` when necessary.

Internally, `device.Detect` delegates to dedicated helpers:

```go
dev, err := device.Detect(ctx, "/dev/sdb", true, "auto", "", "", "", 0, 0, logger)
// detectFileDevice, detectLVMDevice, or detectRawDevice is selected based on the path.
```

Snapshots provide a crash-consistent view of a device. LVM volumes are
snapshotted automatically and removed after transfer. Raw block devices and
regular files do not have a snapshot mechanism; to avoid inconsistent reads you
must either take them offline with `--offline` or freeze the filesystem with
`--fs-freeze-command` and `--fs-thaw-command`. Snapshot creation requires root privileges, so non-root
invocations must permit escalation via `sudo -n`.

Examples:

```sh
lvmsync --source-type lvm /dev/vg0/origin /tmp/dump
lvmsync --dest-type raw dumpfile /dev/sdb
lvmsync --source-type raw --offline /dev/sdb /tmp/dump
lvmsync --source-type raw --fs-freeze-command "fsfreeze -f '/mnt/data dir'" --fs-thaw-command "fsfreeze -u '/mnt/data dir'" /dev/sdb /tmp/dump
```

### Raw device safety

Reading from a live block device can corrupt data if writes occur during the transfer. Ensure a consistent view with one of the following options:

- `--offline` – assert that no process will write to the source device.
- `--fs-freeze-command`/`--fs-thaw-command` – run commands that freeze and thaw the filesystem around the read. Arguments are parsed with shell-style quoting, so wrap paths containing spaces in quotes.
- Time out freeze and thaw helpers with `--freeze-timeout` and `--thaw-timeout` (default `10s`).

Freeze and thaw commands are validated before execution. The command name must match `^[a-zA-Z0-9._-]+$`, be set, free of NUL bytes, every argument must avoid NULs, and the executable must be discoverable in `$PATH`; otherwise lvmsync returns an error.

Example using the provided scripts:

```sh
lvmsync --source-type raw \
  --fs-freeze-command "fsfreeze-freeze.sh /mnt" \
  --fs-thaw-command "fsfreeze-thaw.sh /mnt" \
  /dev/sdb /tmp/dump
```

`docs/fsfreeze-freeze.sh` and `docs/fsfreeze-thaw.sh` demonstrate basic freeze and thaw operations; add the scripts to your `$PATH` to use them.

### Configuration sources and precedence

LVMSync uses [`pflag`](https://github.com/spf13/pflag) and [`viper`](https://github.com/spf13/viper) so every option can be
set via flags, environment variables, or the `config.yaml` file. Values are resolved with the following precedence (highest
first):

1. Command-line flags
2. `LVMSYNC_*` environment variables
3. `config.yaml`
4. Built-in defaults

Environment variables use the flag name in uppercase with underscores, e.g.:

```sh
export LVMSYNC_PARALLEL=8
export LVMSYNC_SSH_USER=backup
```

With a `config.yaml` containing:

```yaml
parallel: 4
```

running `LVMSYNC_PARALLEL=8 lvmsync run --parallel 16` results in `parallel=16`
because flags override environment variables, which override the config file.

Environment variables for the gRPC daemon use the `LVMSYNC_GRPC_` prefix with dashes converted to underscores, for example:

```sh
LVMSYNC_GRPC_GRPC_PORT=9443 LVMSYNC_GRPC_TLS_CERT=cert.pem lvmsync-grpcd
```

### Option reference

| Flag | Environment variable | Config key | Description |
|------|----------------------|------------|-------------|
| `--config` | `LVMSYNC_CONFIG` | `config` | Path to config YAML file |
| `--apply` | `LVMSYNC_APPLY` | `apply` | Apply mode: read change dump from file ('-' for STDIN) and apply to destination device |
| `--stdout` | `LVMSYNC_STDOUT` | `stdout` | Write change dump to STDOUT |
| `--source-type` | `LVMSYNC_SOURCE_TYPE` | `source-type` | Source device type: `auto`, `file`, `raw`, or `lvm` |
| `--dest-type` | `LVMSYNC_DEST_TYPE` | `dest-type` | Destination device type: `auto`, `file`, `raw`, or `lvm` |
| `--offline` | `LVMSYNC_OFFLINE` | `offline` | Assume source raw device is offline |
| `--fs-freeze-command` | `LVMSYNC_FS_FREEZE_COMMAND` | `fs-freeze-command` | Command to freeze filesystem before reading raw source; arguments are split with shell-style quoting and executable name must match `^[a-zA-Z0-9._-]+$` |
| `--fs-thaw-command` | `LVMSYNC_FS_THAW_COMMAND` | `fs-thaw-command` | Command to thaw filesystem after reading raw source; arguments are split with shell-style quoting and executable name must match `^[a-zA-Z0-9._-]+$` |
| `--freeze-timeout` | `LVMSYNC_FREEZE_TIMEOUT` | `freeze_timeout` | Timeout for filesystem freeze command |
| `--thaw-timeout` | `LVMSYNC_THAW_TIMEOUT` | `thaw_timeout` | Timeout for filesystem thaw command |
| `--mode` | `LVMSYNC_MODE` | `mode` | Configuration preset: `default` or `throughput`; unknown modes fail validation |
| `--parallel` | `LVMSYNC_PARALLEL` | `parallel` | Number of concurrent workers |
| `--concurrency` | `LVMSYNC_TRANSPORT_CONCURRENCY` | `concurrency` | Stream concurrency (0 to autotune based on BDP) |
| `--zerocopy` | `LVMSYNC_ZEROCOPY` | `zerocopy` | Enable zero-copy transfers |
| `--odirect` | `LVMSYNC_ODIRECT` | `odirect` | Use O_DIRECT for device I/O when possible |
| `--numa_pin` | `LVMSYNC_NUMA_PIN` | `numa_pin` | Pin worker goroutines to device NUMA node |
| `--max_retries` | `LVMSYNC_MAX_RETRIES` | `max_retries` | Maximum number of retries per block |
| `--resume` | `LVMSYNC_RESUME` | `resume` | Path to resume state file (records dedup mode and last chunk boundary) |
| `--speed` | `LVMSYNC_SPEED` | `speed` | Transfer speed limit |
| `--sync-interval` | `LVMSYNC_SYNC_INTERVAL` | `sync_interval` | Bytes between fdatasync calls (CLI flag: `--sync_interval`) |
| `--checkpoint_bytes` | `LVMSYNC_CHECKPOINT_BYTES` | `checkpoint_bytes` | Bytes between resume checkpoints |
| `--checkpoint_interval` | `LVMSYNC_CHECKPOINT_INTERVAL` | `checkpoint_interval` | Duration between checkpoints |
| `--block_size` | `LVMSYNC_BLOCK_SIZE` | `block_size` | Block size for data transfer; specify 'auto' or 0 for automatic detection |
| `--verbose` | `LVMSYNC_VERBOSE` | `verbose` | Verbosity level |
| `--verify_checksum` | `LVMSYNC_VERIFY_CHECKSUM` | `verify_checksum` | Enable checksum verification |
| `--checksum_algorithm` | `LVMSYNC_CHECKSUM_ALGORITHM` | `checksum_algorithm` | Checksum algorithm: `auto`, `sha256`, `blake3`, or `blake3-512` |
| `--progress` | `LVMSYNC_PROGRESS` | `progress` | Show progress during transfer |
| `--manifest_path` | `LVMSYNC_MANIFEST_PATH` | `manifest_path` | Path to manifest file |
| `--manifest-progress-interval` | `LVMSYNC_MANIFEST_PROGRESS_INTERVAL` | `manifest_progress_interval` | Interval between progress logs during manifest rebuild |
| `--manifest_timeout` | `LVMSYNC_MANIFEST_TIMEOUT` | `manifest_timeout` | Timeout for manifest rebuild (0 disables) |
| `--ssh_host` | `LVMSYNC_SSH_HOST` | `ssh_host` | SSH host |
| `--ssh_user` | `LVMSYNC_SSH_USER` | `ssh_user` | SSH username |
| `--ssh_key` | `LVMSYNC_SSH_KEY` | `ssh_key` | Path to SSH private key |
| `--ssh_host_key_path` | `LVMSYNC_SSH_HOST_KEY_PATH` | `ssh_host_key_path` | Path to SSH host private key |
| `--ssh_agent` | `LVMSYNC_SSH_AGENT` | `ssh_agent` | Use SSH agent for authentication |
| `--ssh_port` | `LVMSYNC_SSH_PORT` | `ssh_port` | SSH port |
| `--ssh_timeout` | `LVMSYNC_SSH_TIMEOUT` | `ssh_timeout` | SSH connection timeout |
| `--ssh_keepalive` | `LVMSYNC_SSH_KEEPALIVE` | `ssh_keepalive` | SSH keepalive interval |
| `--known_hosts` | `LVMSYNC_KNOWN_HOSTS` | `known_hosts` | Path to known_hosts file |
| `--strict_host_key_checking` | `LVMSYNC_STRICT_HOST_KEY_CHECKING` | `strict_host_key_checking` | Require host keys to be present in `known_hosts`; when `false`, host key verification is disabled |
| `--lvmsync_path` | `LVMSYNC_LVMSYNC_PATH` | `lvmsync_path` | Remote command to run (basename sanitized; only `[a-zA-Z0-9._-]+` allowed) |
| `--remote_pre_script` | `LVMSYNC_REMOTE_PRE_SCRIPT` | `remote_pre_script` | Remote script to run before transfer (times out after `ssh_timeout`) |
| `--remote_post_script` | `LVMSYNC_REMOTE_POST_SCRIPT` | `remote_post_script` | Remote script to run after transfer (separate `ssh_timeout`) |
| `--dedup_strategy` | `LVMSYNC_DEDUP_STRATEGY` | `dedup_strategy` | Deduplication strategy: `none`, `auto`, `checksum`, `rolling_hash`, or `bloom` |
| `--dedup_state_file` | `LVMSYNC_DEDUP_STATE_FILE` | `dedup_state_file` | Path to deduplication state file |
| `--cdc-min` | `LVMSYNC_DEDUP_CDC_MIN` | `cdc_min` | Minimum chunk size for CDC |
| `--cdc-avg` | `LVMSYNC_DEDUP_CDC_AVG` | `cdc_avg` | Target average chunk size for CDC |
| `--cdc-max` | `LVMSYNC_DEDUP_CDC_MAX` | `cdc_max` | Maximum chunk size for CDC |
| `--bloom_entries` | `LVMSYNC_DEDUP_BLOOM_ENTRIES` | `bloom_entries` | Estimated number of entries for bloom filter |
| `--bloom_fp_rate` | `LVMSYNC_DEDUP_BLOOM_FP_RATE` | `bloom_fp_rate` | False positive rate for bloom filter |
| `--bloom_mbits` | `LVMSYNC_DEDUP_BLOOM_MBITS` | `bloom_mbits` | Bloom filter m bits power |
| `--compress` | `LVMSYNC_COMPRESSION_COMPRESS` | `compress` | Compression type: `none`, `lz4`, `zstd`, or `auto` |
| `--zstd_level` | `LVMSYNC_COMPRESSION_ZSTD_LEVEL` | `zstd_level` | Zstd compression level (`1-5`) |
| `--lz4_level` | `LVMSYNC_COMPRESSION_LZ4_LEVEL` | `lz4_level` | LZ4 compression level: `fast` or `hc` |
| `--compress_concurrency` | `LVMSYNC_COMPRESSION_COMPRESS_CONCURRENCY` | `compress_concurrency` | Compression concurrency (0 to use `GOMAXPROCS`) |
| `--compress_threshold` | `LVMSYNC_COMPRESSION_COMPRESS_THRESHOLD` | `compress_threshold` | Skip compression when estimated ratio exceeds this value |
| `--skip_snapshot_creation` | `LVMSYNC_SKIP_SNAPSHOT_CREATION` | `skip_snapshot_creation` | Skip automatic snapshot creation |
| `--skip_disk_check` | `LVMSYNC_SKIP_DISK_CHECK` | `skip_disk_check` | Skip disk space check before snapshot creation |
| `--snapshot_size` | `LVMSYNC_SNAPSHOT_SIZE` | `snapshot_size` | Snapshot size (e.g., `20G` or `20%`) |
| `--lvm-escalation` | `LVMSYNC_LVM_ESCALATION` | `lvm_escalation` | Command used to escalate privileges for LVM commands |
| `--lvm_timeout` | `LVMSYNC_LVM_TIMEOUT` | `lvm_timeout` | Timeout for LVM operations |
| `--volume_group` | `LVMSYNC_VOLUME_GROUP` | `volume_group` | Source volume group; derived from the source device path when empty |
| `--target_volume_group` | `LVMSYNC_TARGET_VOLUME_GROUP` | `target_volume_group` | Volume group name of the target LVM volume |
| `--target_vgs` | `LVMSYNC_TARGET_VGS` | `target_vgs` | Candidate target volume groups for auto-selection |
| `--dry-run` | `LVMSYNC_DRY_RUN` | `dry_run` | Print actions without executing |
| `--transport` | `LVMSYNC_TRANSPORT_TRANSPORT` | `transport` | Ordered transports to try (e.g., `quic,h2,tcp+tls,ssh`) |
| `--tcp_port` | `LVMSYNC_TRANSPORT_TCP_PORT` | `tcp_port` | TCP+TLS port |
| `--tcp_parallel` | `LVMSYNC_TRANSPORT_TCP_PARALLEL` | `tcp_parallel` | Number of parallel TCP connections |
| `--tcp_lowat` | `LVMSYNC_TRANSPORT_TCP_LOWAT` | `tcp_lowat` | TCP_NOTSENT_LOWAT in bytes |
| `--grpc_listen` | `LVMSYNC_GRPC_LISTEN` | `grpc_listen` | gRPC listen address |
| `--grpc_connect` | `LVMSYNC_GRPC_CONNECT` | `grpc_connect` | gRPC server address to connect to |
| `--grpc_port` | `LVMSYNC_GRPC_PORT` | `grpc_port` | gRPC port to listen on |
| `--grpc_dial_timeout` | `LVMSYNC_GRPC_DIAL_TIMEOUT` | `grpc_dial_timeout` | gRPC dial timeout |
| `--grpc_setup_timeout` | `LVMSYNC_GRPC_SETUP_TIMEOUT` | `grpc_setup_timeout` | gRPC setup timeout |
| `--grpc_heartbeat_interval` | `LVMSYNC_GRPC_HEARTBEAT_INTERVAL` | `grpc_heartbeat_interval` | gRPC heartbeat interval |
| `--grpc_heartbeat_send_timeout` | `LVMSYNC_GRPC_HEARTBEAT_SEND_TIMEOUT` | `grpc_heartbeat_send_timeout` | gRPC heartbeat send timeout |
| `--keepalive_time` | `LVMSYNC_GRPC_KEEPALIVE_TIME` | `keepalive_time` | Interval between server pings |
| `--keepalive_timeout` | `LVMSYNC_GRPC_KEEPALIVE_TIMEOUT` | `keepalive_timeout` | Wait for ping ack before closing |
| `--request_timeout` | `LVMSYNC_GRPC_REQUEST_TIMEOUT` | `request_timeout` | Deadline for unary RPC handlers |
| `--tls_cert` | `LVMSYNC_TLS_CERT` | `tls_cert` | TLS certificate file |
| `--tls_key` | `LVMSYNC_TLS_KEY` | `tls_key` | TLS key file |
| `--ca_cert` | `LVMSYNC_CA_CERT` | `ca_cert` | CA certificate file |
| `--allow_insecure` | `LVMSYNC_ALLOW_INSECURE` | `allow_insecure` | Allow insecure (no TLS) |

If `--ssh_key` is empty, lvmsync contacts the SSH agent referenced by `SSH_AUTH_SOCK`. The agent connection uses `--ssh_timeout` as its deadline.
SSH transport negotiation also derives read and write deadlines from the caller's context; when the context expires, the handshake fails quickly and deadlines are cleared afterward.

### Common deployment scenarios

- **Local disk to disk**:

  ```sh
  lvmsync run /dev/vg0/source /dev/vg0/backup
  ```

- **Remote over SSH**:

  ```sh
  lvmsync run /dev/vg0/source user@backup:/dev/vg1/target --ssh_key ~/.ssh/id_ed25519
  ```

- **Using the gRPC control plane**:

  ```sh
  lvmsync run --grpc_connect backup:9443 /dev/vg0/source /dev/vg1/target
  ```

- **Throughput-optimized transfer**:

  ```sh
  lvmsync run --mode throughput /dev/vg0/source /dev/vg1/target
  ```

## gRPC Control Plane

The optional gRPC daemon exposes snapshot management and replication over a mutually authenticated channel. Plaintext connections are rejected unless `--allow-insecure` is explicitly set.

TLS mode requires explicit certificates. Provide `--tls-cert`, `--tls-key`, and `--ca-cert`; the daemon fails to start if any are missing and does not generate self-signed certificates.

To detect stalled clients, the daemon sends periodic keepalive pings governed by
`--keepalive-time` (default `2m`). If an acknowledgement is not received within
`--keepalive-timeout` (default `20s`), the connection is closed. Unary RPCs are
wrapped with a deadline controlled by `--request-timeout` (default `15s`).

`StartGRPCServer` accepts a `context.Context` and runs the server in a goroutine, returning a buffered error channel. Cancel the context or invoke the cleanup function to stop the server and wait on the channel during shutdown to surface any serve errors.

1. **Handshake** – clients advertise `sector_size`, `alignment`, `max_concurrency`, and whether deduplication and compression are supported.
2. **Session Creation** – the client sends an ephemeral certificate and receives a session ID, server certificate, and pre-shared key.
3. **Resume Bitmap** – dirty block bitmaps are streamed with the session ID to resume interrupted transfers, and final manifests carrying SHA-256 digests validate completion.
4. **Ack/Ping Stream** – a bidirectional stream of `Ack` messages per session provides keep-alives and progress confirmation.
5. **Finalization** – the client requests completion using the session ID when replication is done.

Configuration comes from flags, `LVMSYNC_GRPC_*` environment variables, or a YAML file with flags taking precedence.

Run the daemon with TLS:

```sh
lvmsync-grpcd --grpc-port 9443 --tls-cert cert.pem --tls-key key.pem --ca-cert ca.pem
```

Disabling TLS with `--allow-insecure` is supported for development but is unsafe for production deployments.

On failure, `lvmsync-grpcd` logs the error and exits with status `1` so calling scripts can inspect `$?`.

Environment variables provide the same settings:

```sh
LVMSYNC_GRPC_GRPC_PORT=9443 \
LVMSYNC_GRPC_TLS_CERT=cert.pem \
LVMSYNC_GRPC_TLS_KEY=key.pem \
LVMSYNC_GRPC_CA_CERT=ca.pem \
lvmsync-grpcd
```

Misconfiguration logs an error and exits with code `1`:

```sh
lvmsync-grpcd --tls-cert missing --tls-key missing
{"level":"error","msg":"init gRPC server","error":"load TLS key pair: open missing: no such file or directory"}
echo $?
1
```

YAML (`grpcd.yaml`):

```yaml
grpc-port: 9443
tls-cert: cert.pem
tls-key: key.pem
ca-cert: ca.pem
```

Clients connect using `--grpc_connect`:

```sh
lvmsync run --grpc_connect localhost:9443 /dev/vg0/snap0 /dev/vg0/data
```

```sh
LVMSYNC_GRPC_CONNECT=localhost:9443 lvmsync run /dev/vg0/snap0 /dev/vg0/data
```

### `config.yaml` example

```yaml
parallel: 4               # General Options
ssh_host: backup          # SSH Options
ssh_user: backup          # SSH Options
remote_pre_script: pre.sh # Remote Options
dedup_strategy: bloom     # Deduplication Options
compress: zstd            # Compression Options
snapshot_size: 20%        # LVM Options
grpc_listen: ":8443"      # gRPC Options
```

Use `--config` to point to a different file.

### Invocation examples

With flags:

```sh
lvmsync run --parallel 8 --snapshot_size 10% /dev/vg0/snap0 /mnt/backup
```

With environment variables:

```sh
LVMSYNC_PARALLEL=8 LVMSYNC_SNAPSHOT_SIZE=10% lvmsync run /dev/vg0/snap0 /mnt/backup
```

With a config file:

```sh
lvmsync run --config config.yaml /dev/vg0/snap0 /mnt/backup
```

## Transport Registry

Transport selection is controlled by the `--transport` flag, which accepts a comma-separated ordered list of
transports to attempt (for example `quic,h2,tcp+tls,ssh`). The `quic` transport runs over TLS 1.3 with mutual
authentication, negotiates the `lvmsync` ALPN, and exposes both bidirectional streams and datagrams. The `h2`
transport also requires TLS 1.3 with client certificates and negotiates the `h2` ALPN. Provide certificates via
`--tls_cert`, `--tls_key`, and `--ca_cert`. TLS transports require a trusted CA certificate and will refuse
connections when no roots are provided unless `--allow_insecure` (or the `AllowInsecure` configuration flag) is
set. Enabling this option logs a warning. Client certificates must be supplied explicitly; transports no longer generate self-signed certificates
automatically. The [transport documentation](docs/transports.md) covers each option in depth. The flags below
configure transport behavior.

### Flags and environment variables

| Flag | Environment variable | Description | mTLS |
|------|----------------------|-------------|------||
| `--transport` | `LVMSYNC_TRANSPORT_TRANSPORT` | Ordered transports to try (e.g., `quic,h2,tcp+tls,ssh`) |
| `--concurrency` | `LVMSYNC_TRANSPORT_CONCURRENCY` | Stream concurrency (0 to autotune based on BDP) |
| `--tcp_port` | `LVMSYNC_TRANSPORT_TCP_PORT` | TCP+TLS port |
| `--h2-port` | `LVMSYNC_H2_PORT` | HTTP/2 port |
| `--tcp_parallel` | `LVMSYNC_TRANSPORT_TCP_PARALLEL` | Number of parallel TCP connections |
| `--tcp_lowat` | `LVMSYNC_TRANSPORT_TCP_LOWAT` | TCP_NOTSENT_LOWAT in bytes |
| `--ssh_port` | `LVMSYNC_SSH_PORT` | SSH port |
| `--ssh_port` | `LVMSYNC_SSH_PORT` | SSH port | ❌ |
| `--tls_cert` | `LVMSYNC_TLS_CERT` | TLS certificate file | ✅ |
| `--tls_key` | `LVMSYNC_TLS_KEY` | TLS key file | ✅ |
| `--ca_cert` | `LVMSYNC_CA_CERT` | CA certificate file | ✅ |
| `--tcp_parallel` | `LVMSYNC_TCP_PARALLEL` | Number of parallel TCP connections | n/a |
| `--tcp_lowat` | `LVMSYNC_TCP_LOWAT` | TCP_NOTSENT_LOWAT in bytes | n/a |

### Usage examples

**Multiple transports**

```sh
lvmsync run --transport quic,h2,tcp+tls,ssh --tcp_port 9443 /dev/vg0/snap0 /mnt/backup
```

**QUIC**

```sh
lvmsync run --transport quic --tls_cert cert.pem --tls_key key.pem --ca_cert ca.pem
# or
LVMSYNC_TRANSPORT_TRANSPORT=quic LVMSYNC_TLS_CERT=cert.pem LVMSYNC_TLS_KEY=key.pem LVMSYNC_CA_CERT=ca.pem lvmsync run
```

**TCP+TLS**

```sh
lvmsync run --transport tcp+tls --tcp_port 9443
# or
LVMSYNC_TRANSPORT_TRANSPORT=tcp+tls LVMSYNC_TRANSPORT_TCP_PORT=9443 lvmsync run
```

**HTTP/2**

```sh
lvmsync run --transport h2 --h2-port 9443 --tls_cert cert.pem --tls_key key.pem --ca_cert ca.pem
```

**SSH**

```sh
lvmsync run --transport ssh backup@host:/dev/vg1/target --ssh_port 2222
# or
LVMSYNC_TRANSPORT_TRANSPORT=ssh LVMSYNC_SSH_PORT=2222 lvmsync run backup@host:/dev/vg1/target
```

## Hybrid Deduplication and Adaptive Compression

Hybrid dedup combines fixed-size and content-defined chunking. Enable it with `--dedup hybrid` and tune FastCDC with `--cdc-min`, `--cdc-avg`, and `--cdc-max`.

| Flag (`--cdc-*`) | Environment variable | Config key | Description |
|------------------|----------------------|------------|-------------|
| `--cdc-min`      | `LVMSYNC_DEDUP_CDC_MIN`    | `cdc_min`  | Minimum chunk size |
| `--cdc-avg`      | `LVMSYNC_DEDUP_CDC_AVG`    | `cdc_avg`  | Target average chunk size |
| `--cdc-max`      | `LVMSYNC_DEDUP_CDC_MAX`    | `cdc_max`  | Maximum chunk size |

The three values must be positive and satisfy `--cdc-min ≤ --cdc-avg ≤ --cdc-max`.
LVMSync aborts when the sizes are non-positive or unordered.

The Bloom filter de-duplicates previously seen chunks. Size it with `--bloom_entries` and desired false positive rate via `--bloom_fp_rate`. For an mmap-backed index, `--bloom_mbits` controls the bitmap size in megabits.

Compression samples 8 KiB from each chunk and skips when the estimated ratio exceeds `--compress_threshold`. `--compress auto` selects LZ4 for chunks under 256 KiB and Zstd for larger chunks when AVX2 or NEON is available, falling back to LZ4 otherwise.

CLI:

```sh
lvmsync run --dedup hybrid --cdc-min 4096 --cdc-avg 65536 --cdc-max 1048576 /dev/vg0/snap0 /mnt/backup
```

Environment:

```sh
LVMSYNC_DEDUP=hybrid \
LVMSYNC_DEDUP_CDC_MIN=4096 \
LVMSYNC_DEDUP_CDC_AVG=65536 \
LVMSYNC_DEDUP_CDC_MAX=1048576 \
lvmsync run /dev/vg0/snap0 /mnt/backup
```

YAML:

```yaml
dedup: hybrid
cdc_min: 4096
cdc_avg: 65536
cdc_max: 1048576
```

### Dedup configuration

The `dedup` package exposes a `LoadConfig` helper that reads tuning parameters
from flags, `LVMSYNC_*` environment variables, or keys in a YAML file. Values
are resolved with the following precedence (highest first):

1. Command-line flags
2. `LVMSYNC_*` environment variables
3. `config.yaml`
4. Built-in defaults

| Flag | Environment variable | Config key | Description | Default |
|------|----------------------|------------|-------------|---------|
| `--min_chunk_size` | `LVMSYNC_MIN_CHUNK_SIZE` | `min_chunk_size` | Minimum chunk size in bytes | `4096` |
| `--max_chunk_size` | `LVMSYNC_MAX_CHUNK_SIZE` | `max_chunk_size` | Maximum chunk size in bytes | `1048576` |
| `--false_positive_rate` | `LVMSYNC_FALSE_POSITIVE_RATE` | `false_positive_rate` | Bloom filter false positive rate | `0.001` |
| `--ram_bytes` | `LVMSYNC_RAM_BYTES` | `ram_bytes` | RAM budget for the Bloom filter | `1073741824` |
| `--volume_size` | `LVMSYNC_VOLUME_SIZE` | `volume_size` | Size of the volume being processed | `0` |
| `--hash_key` | `LVMSYNC_HASH_KEY` | `hash_key` | Optional hex-encoded key for BLAKE3 hashing | `""` |

Two presets are available via `--mode`: `default` and `throughput`. Any other value causes configuration validation to fail.

## Throughput Mode Presets

`--mode throughput` applies a set of options tuned for high-bandwidth links:

- transport order `quic,h2,tcp+tls,ssh`
- concurrency `8`
- deduplication mode `hybrid`
- compression `auto`
- enables `--odirect`

CLI:

```sh
lvmsync run --mode throughput /dev/vg0/snap0 /mnt/backup
```

Environment:

```sh
LVMSYNC_MODE=throughput lvmsync run /dev/vg0/snap0 /mnt/backup
```

YAML:

```yaml
mode: throughput
```

### Logging and progress

Logs are emitted with [zap](https://github.com/uber-go/zap) to stderr. Progress updates are also written to stderr when
`--progress` is enabled (default). Disable them with `--progress=false`.

## Installation

### Requirements

- Go 1.22+
- Linux only (tested on `amd64` and `arm64` architectures)
- LVM2 with development headers providing `liblvm2cmd` (`liblvm2-dev`)
  - A recent LVM2 release providing the modern `liblvm2cmd` API (e.g., 2.03.21+) is required.
- SSH client & server (for remote transfers)

### Installing LVM2 Development Headers

CGO uses `pkg-config` to locate the LVM2 and device-mapper libraries. Install the development headers and
`pkg-config` package on your system:

```sh
# Debian/Ubuntu
sudo apt install -y lvm2 liblvm2-dev pkg-config

# RHEL/CentOS
sudo yum install -y lvm2-devel pkgconfig
```

If the `.pc` files are installed in a non-standard location, set `PKG_CONFIG_PATH` so that `pkg-config` can
find them.

### Build

Clone the repository and build the binary using Go modules with CGO enabled:

```sh
git clone https://github.com/oferchen/lvmsync_go.git
cd lvmsync_go
go mod tidy
CGO_ENABLED=1 go build -o lvmsync .
```

To build on systems without LVM2, disable CGO. This uses stub implementations and omits LVM features:

```sh
CGO_ENABLED=0 go build -o lvmsync .
```

### Makefile

````sh
make proto   # generate gRPC code
make build   # build binaries
make test    # run tests
=======
### systemd Service

Install the gRPC daemon as a systemd service:

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin lvmsync
sudo cp packaging/systemd/lvmsync-grpcd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now lvmsync-grpcd
````

Logs are collected by journald and can be viewed with:

```sh
journalctl -u lvmsync-grpcd
```

## Usage

### Basic Syntax

```sh
lvmsync run [--dry-run] [--transport quic,h2,tcp+tls,ssh] <snapshot|lvm device> <destination>
```

The tool supports both local and remote transfers, as well as an "apply mode" for applying change dumps. Use `--dry-run` to print planned actions without executing and `--transport` to provide an ordered list of transports to try.

## Resume, Manifest, and Verify

Run an initial transfer and write a manifest for later verification or
incremental runs:

```sh
lvmsync run --manifest_path snapshot.manifest /dev/vg0/source /dev/vg1/target
```

See the [manifest documentation](docs/manifest.md) for details on the binary
format and rebuild options.

Resume an interrupted transfer using a checkpointed state file:

```sh
lvmsync run --resume statefile /dev/vg0/snap0 /dev/vg0/data
```

Rebuild a manifest index for an existing device:

```sh
lvmsync manifest rebuild /dev/vg0/lv0
```
Progress logs are emitted every 10s by default; adjust with `--manifest-progress-interval`.
The command times out after 1m unless overridden with `--manifest_timeout` (0 disables).
Rebuild refuses to run if the device is mounted read-write; pass `--manifest-allow-mounted` to override.
Mount detection parses `/proc/self/mountinfo` using `github.com/moby/sys/mountinfo` to correctly handle devices with spaces or special characters.
Rebuild fails if the device reports a block size of 0.

Manifests embed a persistent device identifier in a fixed 64-byte field. The
`manifest rebuild` command fails if the identifier exceeds this limit.

Verify that a source and destination match:

```sh
lvmsync verify /dev/vg0/source /dev/vg1/target
```

Supply options such as block size or deduplication mode to control how data is compared. For example, to estimate verification without reading data:

```sh
lvmsync verify --dry-run /dev/vg0/source /dev/vg1/target
```

To verify using 4 KiB blocks and a manifest generated earlier:

```sh
lvmsync verify --block_size 4K /dev/vg0/source /dev/vg1/target
```

Flags are parsed via Viper, so the same settings can be provided through
`LVMSYNC_*` environment variables or a `config.yaml` file.

### Options

#### General Options

| Option              | Description                                                                                             | Default   |
| ------------------- | ------------------------------------------------------------------------------------------------------- | --------- |
| `--config`          | Path to a YAML configuration file                                                                       | `""`      |
| `--apply`           | Apply mode: read change dump from file (`-` for STDIN) and apply to destination device                  | `""`      |
| `--stdout`          | Write change dump to STDOUT                                                                             | `false`   |
| `--parallel`        | Number of concurrent workers                                                                            | `4`       |
| `--zerocopy`        | Enable zero-copy transfers (only used in sequential mode)                                               | `false`   |
| `--max_retries`     | Maximum number of retries per block                                                                     | `3`       |
| `--resume`          | Path to resume state file                                                                               | `""`      |
| `--speed`           | Transfer speed limit (e.g., `"100MB"`)                                                                  | `"100MB"` |
| `-v, --verbose`     | Verbosity level (e.g., `-v`, `-vv`, `-vvv`)                                                             | `0`       |
| `--verify_checksum` | Enable checksum verification for data integrity                                                         | `false`   |
| `--progress`        | Show progress percentage during the transfer                                                            | `true`    |
| `--block_size`      | Block size for data transfer (e.g., `"4K"`, `"64K"`, `"512K"`, `"1M"`), use `0` for automatic detection | `"4K"`    |
| `--dry-run`         | Print actions without executing | `false`   |
| `--offline`         | Assume source raw device is offline | `false`   |
| `--fs-freeze-command` | Command to freeze filesystem before reading raw source; arguments use shell-style quoting | `""` |
| `--fs-thaw-command`  | Command to thaw filesystem after reading raw source; arguments use shell-style quoting | `""` |
| `--freeze-timeout`   | Timeout for filesystem freeze command | `10s` |
| `--thaw-timeout`     | Timeout for filesystem thaw command | `10s` |
| `--transport`       | Ordered transports to try (e.g., `quic,h2,tcp+tls,ssh`) | `""`      |

#### SSH Options

| Option                    | Description                                                     | Default                  |
| ------------------------- | --------------------------------------------------------------- | ------------------------ |
| `--ssh_host`              | SSH host                                                        | `"localhost"`            |
| `--ssh_user`              | SSH username                                                    | `"root"`                 |
| `--ssh_key`               | Path to SSH private key                                          | `""`                     |
| `--ssh_host_key_path`     | Path to SSH host private key (generates one if empty)            | `""`                     |
| `--ssh_agent`             | Use the SSH agent for authentication                            | `false`                  |
| `--ssh_port`              | SSH port number                                                 | `22`                     |
| `--known_hosts`           | Path to known_hosts file (defaults to `$HOME/.ssh/known_hosts`) | `$HOME/.ssh/known_hosts` |
| `--strict_host_key_checking` | Require host keys to be present in `known_hosts`; when `false`, host key verification is disabled | `true`                   |
| `--ssh_host_key`          | Expected SSH host public key (authorized_keys format)          | `""`                    |

Unknown hosts are rejected unless their keys are present in `known_hosts` or match `--ssh_host_key`.
The host key can also be supplied via `LVMSYNC_SSH_HOST_KEY_PATH` or the `ssh_host_key_path` YAML option.

Programmatic use of the SSH transport requires a configuration populated with
fields like `SSHUser`, `SSHKeyPath`, `HostKeyPath`, `SSHUseAgent`, `SSHPort`, `KnownHosts`,
`StrictHostKeyCheck`, `SSHTimeout`, `SSHKeepAliveInterval`, and `MaxRetries`.
The constructor also requires a `*zap.Logger`:

```
logger, _ := zap.NewProduction()
defer logger.Sync()
sender, receiver, err := ssh.New(cfg, logger)
```

#### Remote Options

The `--lvmsync_path` value is sanitized to its basename and must match
`[a-zA-Z0-9._-]+` to prevent shell injection.

| Option                 | Description                                       | Default     |
| ---------------------- | ------------------------------------------------- | ----------- |
| `--lvmsync_path`       | Remote command to run (sanitized basename)         | `"lvmsync"` |
| `--remote_pre_script`  | Remote script to run before starting the transfer (`ssh_timeout` applies) | `""`        |
| `--remote_post_script` | Remote script to run after finishing the transfer (uses a fresh `ssh_timeout`) | `""`        |

Both scripts are run with the configured `ssh_timeout`. The post script uses its
own timeout and still attempts to execute even when the main transfer fails;
timeouts or cancellations are reported separately.

#### Deduplication Options

| Option               | Description                                                                                             | Default            |
| -------------------- | ------------------------------------------------------------------------------------------------------- | ------------------ |
| `--dedup`            | Deduplication mode ("fixed", "cdc", or "hybrid")                                                  | "fixed"          |
| `--cdc-min`          | Minimum chunk size for CDC                                                                              | 4096             |
| `--cdc-avg`          | Average chunk size for CDC                                                                              | 65536            |
| `--cdc-max`          | Maximum chunk size for CDC                                                                              | 1048576          |
| `--dedup_strategy`   | Deduplication strategy ("none", "auto", "checksum", "rolling_hash", or "bloom"); use `none` to disable | "none"           |
| `--dedup_state_file` | Path to deduplication state file                                                                        | ~/.lvmsync_dedup |
| `--bloom_entries`    | Estimated number of entries for bloom filter                                                            | 1000000          |
| `--bloom_fp_rate`    | False positive rate for bloom filter                                                                    | 0.01             |
| `--bloom_mbits`      | Size of Bloom filter bitmap in megabits (mmap index)                                                   | 0                |

#### Compression Options

| Option                   | Description                                                       | Default  |
| ------------------------ | ----------------------------------------------------------------- | -------- |
| `--compress`             | Compression type (options: `"none"`, `"lz4"`, `"zstd"`, `"auto"`) | `"auto"` |
| `--zstd_level`           | Zstd compression level (`1-5`)                                   | `1`      |
| `--lz4_level`            | LZ4 compression level: `fast` or `hc`                            | `fast`   |
| `--compress_concurrency` | Number of goroutines used for compression (`0` to use all cores)  | `0`      |
| `--compress_threshold`   | Skip compression when estimated ratio exceeds this value         | `0.9`    |

#### LVM Options

| Option | Description | Default |
| ------ | ----------- | ------- |
| `--skip_snapshot_creation` | Skip automatic snapshot creation | `false` |
| `--skip_disk_check` | Skip disk space check before snapshot creation | `false` |
| `--snapshot_size` | Snapshot size as an absolute value (e.g., "20G") or as a percentage (e.g., "20%") | "20%" |
| `--volume_group` | Source volume group. Derived from the source device path when empty | "" |
| `--target_volume_group` | Volume group name of the target LVM volume | "" |
| `--target_vgs` | Candidate target volume groups for auto-selection | [] |
| `--lvm-escalation` | Command used to re-execute the program with elevated privileges when not running as root (e.g., "sudo -n") | "sudo -n" |
| `--lvm_timeout` | Timeout for LVM operations | 10s |

#### gRPC Options

The client aborts dialing if a connection cannot be established within
`--grpc_dial_timeout`, which defaults to `5s`.

| Option             | Description                  | Default         |
| ------------------ | ---------------------------- | --------------- |
| `--grpc_port`      | gRPC port to listen on       | `8443`          |
| `--grpc_dial_timeout` | gRPC dial timeout          | `5s`            |
| `--grpc_setup_timeout` | gRPC setup timeout        | `10s`           |
| `--grpc_heartbeat_interval` | gRPC heartbeat interval | `30s`          |
| `--grpc_heartbeat_send_timeout` | gRPC heartbeat send timeout | `5s` |
| `--keepalive_time` | Interval between server pings | `2m` |
| `--keepalive_timeout` | Timeout waiting for keepalive ack | `20s` |
| `--request_timeout` | Deadline for unary RPCs | `15s` |
| `--tls_cert`       | TLS certificate file         | `""`            |
| `--tls_key`        | TLS key file                 | `""`            |
| `--ca_cert`        | CA certificate file          | `""`            |
| `--allow_insecure` | Allow insecure (disable TLS) | `false`         |

### Examples

#### Local Transfer

Transfer changes from a snapshot to a destination device locally:

```sh
lvmsync run /dev/vg0/snap0 /dev/vg0/data
```

#### Remote Transfer

Replicate data to a remote host. The destination must be specified in `host:device` format (optionally including a username, e.g., `user@host:/dev/vg0/data`):

```sh
lvmsync run /dev/vg0/snap0 user@remote:/dev/vg0/data
```

#### Applying Changes

Apply a change dump to a destination device (read from a file or STDIN):

```sh
lvmsync run --apply dumpfile.lvm /dev/vg0/data
```

#### Using Compression

Estimate a sample of each chunk and compress only when it's worthwhile:

```sh
lvmsync run --compress auto --zstd_level 2 --compress_threshold 0.85 /dev/vg0/snap0 /dev/vg0/data
```

#### Rate Limiting

Transfers can be throttled using a token bucket accurate to ±3% of the target.
Each writer has its own limiter, so multiple transfers with different limits run independently.
Limit the transfer speed to 50MB/s:

```sh
lvmsync run --speed 50MB /dev/vg0/snap0 /dev/vg0/data
```

#### Resuming a Transfer

Resume an interrupted transfer using a resume state file. The file records the last chunk boundaries and digests for fixed, CDC, and hybrid modes. Progress is checkpointed every `--checkpoint-bytes` or `--checkpoint-interval`, and the resume file is removed on successful completion. Changing the transport, compression, checksum algorithm, or dedup mode invalidates the checkpoint:

```sh
lvmsync run --resume statefile /dev/vg0/snap0 /dev/vg0/data
```

#### Full LVM Operation Example

```sh
lvmsync run --skip_disk_check=false --snapshot_size "25%" --volume_group "vg_data" --lvm-escalation "sudo -n" /dev/vg_data/original /dev/vg_data/destination
```

In this example, LVMSync will:

- Validate that the volume group `vg_data` exists.
- Create a snapshot of `/dev/vg_data/original` sized at 25% of the original volume.
  - Automatically re-exec with `sudo -n` if not running as root.
- Monitor snapshot usage (failing fast if usage exceeds 80%).
- Perform the block-level transfer.
- Remove the snapshot upon completion.
- Clean up gracefully if interrupted.

## Manifest Rebuild and Verification

Rebuild a manifest for an existing device when the index is missing or stale:

```sh
lvmsync manifest rebuild /dev/vg0/lv0
```
Progress logs are emitted every 10s by default; adjust with `--manifest-progress-interval`.
The command times out after 1m unless overridden with `--manifest_timeout` (0 disables).

Compare source and destination devices against a manifest:

```sh
lvmsync verify /dev/vg0/snap0 /mnt/backup
```

Use `--dry-run` with `verify` to inspect planned operations without modifying the destination:

```sh
lvmsync verify --dry-run /dev/vg0/source /dev/vg1/target
```

## Configuration Sources

LVMSync binds its command line flags to [Viper](https://github.com/spf13/viper), allowing configuration through flags, environment variables, or a YAML file. The resolution order is:

1. command line flags
2. environment variables (`LVMSYNC_*`)
3. the configuration file (default: `config.yaml`)

Environment variables use the `LVMSYNC_` prefix and match flag names converted to upper case with hyphens replaced by underscores. The `--config` flag can point to an alternative YAML file.

### Examples by Option Group

#### General

CLI:

```sh
lvmsync run --parallel 8 --resume statefile /dev/vg0/snap0 /dev/vg0/data
```

Environment:

```sh
LVMSYNC_PARALLEL=8 LVMSYNC_RESUME=statefile lvmsync run /dev/vg0/snap0 /dev/vg0/data
```

YAML (`config.yaml`):

```yaml
parallel: 8
resume: statefile
```

#### SSH

CLI:

```sh
lvmsync run --ssh_user backup --ssh_port 2222 /dev/vg0/snap0 backup:/dev/vg0/data
```

Environment:

```sh
LVMSYNC_SSH_HOST=backup LVMSYNC_SSH_USER=backup LVMSYNC_SSH_PORT=2222 lvmsync run /dev/vg0/snap0 /dev/vg0/data
```

YAML:

```yaml
ssh_host: backup
ssh_user: backup
ssh_port: 2222
```

#### Remote

CLI:

```sh
lvmsync run --lvmsync_path /usr/bin/lvmsync --remote_pre_script /tmp/pre.sh /dev/vg0/snap0 user@host:/dev/vg0/data
```

Environment:

```sh
LVMSYNC_LVMSYNC_PATH=/usr/bin/lvmsync LVMSYNC_REMOTE_PRE_SCRIPT=/tmp/pre.sh lvmsync run /dev/vg0/snap0 user@host:/dev/vg0/data
```

YAML:

```yaml
lvmsync_path: /usr/bin/lvmsync
remote_pre_script: /tmp/pre.sh
```

#### Deduplication

CLI:

```sh
lvmsync run --dedup_strategy bloom --dedup_state_file ~/.lvmsync_state /dev/vg0/snap0 /dev/vg0/data
```

Environment:

```sh
LVMSYNC_DEDUP_STRATEGY=bloom LVMSYNC_DEDUP_STATE_FILE=~/.lvmsync_state lvmsync run /dev/vg0/snap0 /dev/vg0/data
```

YAML:

```yaml
dedup_strategy: bloom
dedup_state_file: ~/.lvmsync_state
```

LVMSync automatically reloads this state file on startup. Delete it to reset deduplication: `rm ~/.lvmsync_dedup`.

#### Compression

LVMSync samples 8 KiB from each chunk to gauge compression efficiency. If the
compressed sample ratio is greater than or equal to `--compress_threshold`, the
chunk is sent uncompressed. In `auto` mode, chunks smaller than 256 KiB use LZ4,
and larger ones select Zstd (levels 1–3) when AVX2 or NEON is available;
otherwise LZ4.
Levels can be tuned with `--zstd_level` (1-5) or `--lz4_level` (`fast` or `hc`).

CLI:

```sh
lvmsync run --compress auto --zstd_level 2 --compress_threshold 0.85 /dev/vg0/snap0 /dev/vg0/data
```

Environment:

```sh
LVMSYNC_COMPRESSION_COMPRESS=auto LVMSYNC_COMPRESSION_ZSTD_LEVEL=2 LVMSYNC_COMPRESSION_COMPRESS_THRESHOLD=0.85 lvmsync run /dev/vg0/snap0 /dev/vg0/data
```

YAML:

```yaml
compress: auto
zstd_level: 2
compress_threshold: 0.85
```

#### LVM

CLI:

```sh
lvmsync run --snapshot_size 25% --volume_group vg_data /dev/vg_data/original /dev/vg_data/destination
```

Environment:

```sh
LVMSYNC_SNAPSHOT_SIZE=25% LVMSYNC_VOLUME_GROUP=vg_data lvmsync run /dev/vg_data/original /dev/vg_data/destination
```

YAML:

```yaml
snapshot_size: "25%"
volume_group: vg_data
```

#### gRPC

`lvmsync-grpcd` accepts configuration via flags, environment variables prefixed with `LVMSYNC_GRPC_`, or a YAML file. Values are resolved in the following order: flags override environment variables, which override the config file.

| Flag | Environment variable | Config key | Description |
|------|----------------------|------------|-------------|
| `--grpc-port` | `LVMSYNC_GRPC_GRPC_PORT` | `grpc-port` | gRPC port to listen on |
| `--tls-cert` | `LVMSYNC_GRPC_TLS_CERT` | `tls-cert` | TLS certificate file |
| `--tls-key` | `LVMSYNC_GRPC_TLS_KEY` | `tls-key` | TLS key file |
| `--ca-cert` | `LVMSYNC_GRPC_CA_CERT` | `ca-cert` | CA certificate file |
| `--allow-insecure` | `LVMSYNC_GRPC_ALLOW_INSECURE` | `allow-insecure` | Allow insecure (no TLS) |
| `--config` | `LVMSYNC_GRPC_CONFIG` | `config` | Path to config YAML file |

Precedence example:

```sh
cat >grpcd.yaml <<'EOF'
grpc-port: 1111
EOF
export LVMSYNC_GRPC_GRPC_PORT=2222
lvmsync-grpcd --config grpcd.yaml --grpc-port 3333
# effective port: 3333
```

CLI:

```sh
lvmsync-grpcd --grpc-port 9443 --tls-cert cert.pem
```

Environment:

```sh
LVMSYNC_GRPC_GRPC_PORT=9443 LVMSYNC_GRPC_TLS_CERT=cert.pem lvmsync-grpcd
```

YAML (`grpcd.yaml`):

```yaml
grpc-port: 9443
tls-cert: cert.pem
```

Use `--config` to provide an alternate config file path.

## Configuration Validation

Before starting, LVMSync validates key configuration parameters:

- Verifies that the specified volume groups exist.
- Ensures the escalation command is available if not running as root.

Invalid configurations will cause the tool to abort with a clear error message.

## Exit Codes

LVMSync commands such as `lvmsync` and `lvmsync-grpcd` return conventional exit codes to indicate overall status:

| Code | Meaning |
|------|---------|
| `0`  | Success |
| `1`  | Configuration or runtime failure |
| other | Subcommand-specific codes (see individual command docs) |

Exit code handling lives in [main.go](main.go#L21-L37), with configuration errors bubbling up from [cmd/root/root.go](cmd/root/root.go#L42-L52) and runtime errors from [cmd/root/root.go](cmd/root/root.go#L121-L186). Subcommands may return additional exit codes to communicate their own failure modes.

Shell scripts can rely on `set -e` to abort on non-zero exit codes:

```sh
#!/bin/sh
set -e
lvmsync "$@"
echo "transfer completed successfully"
```

## Credits

LVMSync is written in Go by Ofer Chen, inspired by [mpalmer/lvmsync](https://github.com/mpalmer/lvmsync).

## Contributing

Contributions are welcome. See [AGENTS.md](AGENTS.md) for detailed contributor instructions and open TODO items. Please follow the project's coding guidelines, include appropriate logging and error handling, and update documentation as needed.

### Single-Responsibility Functions

Keep functions and packages focused on a single task to simplify maintenance and testing. Break up large components when behavior grows to preserve clarity.

### Dependency Injection

Decouple modules by injecting dependencies through interfaces or constructor parameters. This approach makes components easier to test and swap during refactoring.

For example, the `privilege` package exposes an `Escalator` interface so tests
can stub command execution:

```
esc := privilege.New()
if err := esc.Ensure(); err != nil {
    // handle missing capabilities or sudo
}
cmd := esc.Command("lvs", "--version")
_ = cmd
```

### Test Coverage

Every change should include unit tests. Run `go test -cover ./...` to ensure coverage remains high and regressions are caught early.

Compression detection uses benchmark-driven selection between LZ4 and Zstd and now includes dedicated tests verifying algorithm choice and cache resets.

### Production readiness

- Structured logging uses [`zap`](https://github.com/uber-go/zap); always defer a logger sync to flush entries.
- Configuration is parsed with [`pflag`](https://github.com/spf13/pflag) and [`viper`](https://github.com/spf13/viper). Every option can be set via CLI flags, `LVMSYNC_*` environment variables, or the `config.yaml` file.
- Related flags are organized into thematic `FlagSet`s for concise help output.
- Each function in the codebase includes unit tests covering both success and failure paths.
- Before sending patches, run `go build ./...`, `go test -cover ./...`, and `golangci-lint run`.

## Development

### Development Setup

The Super-Linter workflow validates the entire repository.

### Linting

The `.golangci.yml` config uses standard Go formatters such as `gci`, `gofmt`, `gofumpt`, `goimports`, and `golines`. A misconfigured `swaggo` formatter entry was removed.
Run the linter locally to mirror CI:

```sh
golangci-lint run ./...
```

### Testing

Run unit tests with coverage:

```sh
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Some privileged tests are skipped when run without root access.

The workflow enforces a minimum of 50% total coverage.

## License

GPLv3 License. See the `LICENSE` file for details.
