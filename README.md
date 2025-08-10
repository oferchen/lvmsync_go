# LVMSync
[![Build Status](https://github.com/oferchen/lvmsync_go/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/oferchen/lvmsync_go/actions/workflows/go.yml)
[![Build Status](https://github.com/oferchen/lvmsync_go/actions/workflows/super-linter.yml/badge.svg?branch=main)](https://github.com/oferchen/lvmsync_go/actions/workflows/super-linter.yml)

LVMSync is a high-performance incremental data replication tool for LVM snapshots. It efficiently transfers only changed blocks using metadata from snapshot COW (Copy-On-Write) devices and communicates with LVM through native Go bindings rather than shell commands.

## Features

- **Incremental Block-Level Synchronization**: Transfers only changed blocks.
- **Zero-Copy Transfers**: Utilizes `splice()` for efficient data movement.
- **Parallel Execution**: Configurable concurrency for optimal performance.
- **Rate-Limiting**: Control bandwidth usage during transfers.
- **Compression**: Samples 8 KiB per chunk, skipping compression when the ratio exceeds a threshold. Auto mode selects LZ4 for chunks <256 KiB and Zstd level 1 for larger chunks on AVX2-capable CPUs.
- **Checksum Verification**: Ensures data integrity using SHA-256 or BLAKE3.
- **Native LVM2 Integration**: Uses Go bindings to `liblvm2cmd` instead of shelling out.
- **Deduplication Strategies**: Detect unchanged blocks using checksum, rolling hash, or a Bloom filter with optional FastCDC content-defined chunking and mmap-backed index.
- **Remote Execution via SSH**: Replicates data over SSH with support for pre/post-scripts.
- **Resume Support**: Ability to resume interrupted transfers.
- **LVM Snapshot Management**:
  - Automatic snapshot creation and removal.
  - Configurable snapshot size (absolute or percentage-based).
  - Configurable volume group for constructing the snapshot device path.
  - Auto-selection of target volume groups with sufficient free space.
  - Automatic privilege escalation (defaulting to `sudo -n`).
  - Snapshot health monitoring that fails fast if usage exceeds a threshold.
- **Graceful Shutdown**: Signal handling ensures snapshots are cleaned up on interruption.
- **Flexible Configuration**: Flags, environment variables, or `config.yaml`. See [Configuration](#configuration).
- **Configuration Validation**: Checks key parameters (e.g., volume group existence, escalation command) before starting operations.

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
- `transfer` – performs block-level synchronization, compression, deduplication, and resume logic.
  - Internally split into focused modules: `progress.go`, `handshake.go`, and `block_writer.go` for clearer responsibilities.
- `remote` – wraps SSH functionality for running commands on remote hosts and coordinating transfers.
- `config` – parses and validates configuration files and CLI options.
- `dedup` – houses Bloom filter helpers, chunking logic, and other deduplication utilities.
- `grpc` – provides the gRPC server and authentication helpers used by the remote daemon.
- `common` and `internal` – shared helpers and internal utilities such as multi-error handling.
- `internal/client` – coordinates snapshot preparation and client transfer execution.
- `cmd/lvmsync` – CLI orchestrator with a `signals` subpackage for signal handling and cleanup.
- `cmd/grpcd` – standalone gRPC daemon exposing LVMSync operations remotely.

This structure allows individual packages to be developed and tested in isolation.

### Refactoring Notes

- Snapshot preparation helpers (`ensureVolumeGroups`, `checkDiskSpaceForSnapshot`, `createSnapshotIfNeeded`, `PrepareSnapshot`) and client execution logic are consolidated under `internal/client`.
- These helpers no longer rely on global variables; configuration and loggers are passed explicitly.
- `main.go` now orchestrates operations through the `internal/client` package.

## Logging

LVMSync emits structured logs using [zap](https://github.com/uber-go/zap). Errors are logged with
structured fields instead of being written to stderr, and the logger is flushed on shutdown to
ensure all entries are persisted. When `--progress` is enabled, progress updates are emitted as
structured log entries, allowing external tooling to track transfer completion.

```go
logger, _ := zap.NewProduction()
defer logger.Sync()
start := time.Now()

logger.Info("snapshot complete",
    zap.String("source_path", src),
    zap.String("dest_path", dst),
    zap.Int64("duration_ms", time.Since(start).Milliseconds()),
)
```

Field keys use `snake_case` and include units where appropriate, and `Sync()` is deferred to flush
buffers on exit.

## Configuration

LVMSync uses [`pflag`](https://github.com/spf13/pflag) and [`viper`](https://github.com/spf13/viper) to accept options from
flags, environment variables, and a YAML file.

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

Internally, each group is set up through a dedicated helper such as
`initGeneralFlags`, `initSSHFlags`, or `initCompressionFlags`, keeping flag
definitions focused and easy to maintain.

Example:

```sh
lvmsync --transport quic,h2,tcp+tls,ssh --quic-listen :9000 --tcp-port 9443
```

### Modes

LVMSync exposes a `--mode` flag to apply preset configurations.

- `default` – standard behavior.
- `throughput` – tunes defaults for maximum throughput:
  - transport order `quic,h2,tcp+tls,ssh`
  - concurrency `8`
  - deduplication mode `hybrid`
  - compression `auto`
  - enables `--odirect`
  - large sync and checkpoint intervals
  - QUIC congestion control `bbr`

```go
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

This groups related flags once and lets Viper merge values from flags, `LVMSYNC_*` variables, and the
`config.yaml` file.

The overall loading flow works in three stages:

1. `registerFlags()` adds all flag groups to the command line.
2. `buildViper()` binds flags, `LVMSYNC_*` environment variables, and an optional `config.yaml` into a single configuration source.
3. `LoadConfig()` merges those values with built-in defaults and validates the result.

### Precedence and environment variables

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

If `config.yaml` sets `parallel: 4` and the environment contains
`LVMSYNC_PARALLEL=8`, running `lvmsync --parallel 16` will use `parallel=16`
because flags override environment variables, which override the config file.

### Option reference

| Flag | Environment variable | Config key | Description |
|------|----------------------|------------|-------------|
| `--config` | `LVMSYNC_CONFIG` | `config` | Path to config YAML file |
| `--apply` | `LVMSYNC_APPLY` | `apply` | Apply mode: read change dump from file ('-' for STDIN) and apply to destination device |
| `--stdout` | `LVMSYNC_STDOUT` | `stdout` | Write change dump to STDOUT |
| `--parallel` | `LVMSYNC_PARALLEL` | `parallel` | Number of concurrent workers |
| `--zerocopy` | `LVMSYNC_ZEROCOPY` | `zerocopy` | Enable zero-copy transfers |
| `--max_retries` | `LVMSYNC_MAX_RETRIES` | `max_retries` | Maximum number of retries per block |
| `--resume` | `LVMSYNC_RESUME` | `resume` | Path to resume state file |
| `--speed` | `LVMSYNC_SPEED` | `speed` | Transfer speed limit |
| `--block_size` | `LVMSYNC_BLOCK_SIZE` | `block_size` | Block size for data transfer; specify 'auto' or 0 for automatic detection |
| `--verbose` | `LVMSYNC_VERBOSE` | `verbose` | Verbosity level |
| `--verify_checksum` | `LVMSYNC_VERIFY_CHECKSUM` | `verify_checksum` | Enable checksum verification |
| `--checksum_algorithm` | `LVMSYNC_CHECKSUM_ALGORITHM` | `checksum_algorithm` | Checksum algorithm: `sha256`, `blake3`, or `blake3-512` |
| `--progress` | `LVMSYNC_PROGRESS` | `progress` | Show progress during transfer |
| `--ssh_user` | `LVMSYNC_SSH_USER` | `ssh_user` | SSH username |
| `--ssh_key` | `LVMSYNC_SSH_KEY` | `ssh_key` | Path to SSH private key or use agent |
| `--ssh_port` | `LVMSYNC_SSH_PORT` | `ssh_port` | SSH port |
| `--ssh_timeout` | `LVMSYNC_SSH_TIMEOUT` | `ssh_timeout` | SSH connection timeout |
| `--ssh_keepalive` | `LVMSYNC_SSH_KEEPALIVE` | `ssh_keepalive` | SSH keepalive interval |
| `--known_hosts` | `LVMSYNC_KNOWN_HOSTS` | `known_hosts` | Path to known_hosts file |
| `--stricthostkeychecking` | `LVMSYNC_STRICTHOSTKEYCHECKING` | `strict_host_key_checking` | Enable SSH StrictHostKeyChecking |
| `--lvmsync_path` | `LVMSYNC_LVMSYNC_PATH` | `lvmsync_path` | Remote command to run |
| `--remote_pre_script` | `LVMSYNC_REMOTE_PRE_SCRIPT` | `remote_pre_script` | Remote script to run before transfer |
| `--remote_post_script` | `LVMSYNC_REMOTE_POST_SCRIPT` | `remote_post_script` | Remote script to run after transfer |
| `--dedup_strategy` | `LVMSYNC_DEDUP_STRATEGY` | `dedup_strategy` | Deduplication strategy: `none`, `auto`, `checksum`, `rolling_hash`, or `bloom` |
| `--dedup_state_file` | `LVMSYNC_DEDUP_STATE_FILE` | `dedup_state_file` | Path to deduplication state file |
| `--bloom_entries` | `LVMSYNC_BLOOM_ENTRIES` | `bloom_entries` | Estimated number of entries for bloom filter |
| `--bloom_fp_rate` | `LVMSYNC_BLOOM_FP_RATE` | `bloom_fp_rate` | False positive rate for bloom filter |
| `--compress` | `LVMSYNC_COMPRESS` | `compress` | Compression type: `none`, `lz4`, `zstd`, or `auto` |
| `--zstd_level` | `LVMSYNC_ZSTD_LEVEL` | `zstd_level` | Zstd compression level (`1-5`) |
| `--lz4_level` | `LVMSYNC_LZ4_LEVEL` | `lz4_level` | LZ4 compression level: `fast` or `hc` |
| `--compress_concurrency` | `LVMSYNC_COMPRESS_CONCURRENCY` | `compress_concurrency` | Compression concurrency (0 to use `GOMAXPROCS`) |
| `--compress_threshold` | `LVMSYNC_COMPRESS_THRESHOLD` | `compress_threshold` | Skip compression when estimated ratio exceeds this value |
| `--skip_snapshot_creation` | `LVMSYNC_SKIP_SNAPSHOT_CREATION` | `skip_snapshot_creation` | Skip automatic snapshot creation |
| `--skip_disk_check` | `LVMSYNC_SKIP_DISK_CHECK` | `skip_disk_check` | Skip disk space check before snapshot creation |
| `--snapshot_size` | `LVMSYNC_SNAPSHOT_SIZE` | `snapshot_size` | Snapshot size (e.g., `20G` or `20%`) |
| `--lvm_escalation` | `LVMSYNC_LVM_ESCALATION` | `lvm_escalation` | Command used to escalate privileges for LVM commands |
| `--volume_group` | `LVMSYNC_VOLUME_GROUP` | `volume_group` | Source volume group; derived from the source device path when empty |
| `--target_volume_group` | `LVMSYNC_TARGET_VOLUME_GROUP` | `target_volume_group` | Volume group name of the target LVM volume |
| `--target_vgs` | `LVMSYNC_TARGET_VGS` | `target_vgs` | Candidate target volume groups for auto-selection |
| `--grpc_listen` | `LVMSYNC_GRPC_LISTEN` | `grpc_listen` | gRPC listen address |
| `--grpc_connect` | `LVMSYNC_GRPC_CONNECT` | `grpc_connect` | gRPC server address to connect to |
| `--grpc_port` | `LVMSYNC_GRPC_PORT` | `grpc_port` | gRPC port to listen on |
| `--tls_cert` | `LVMSYNC_TLS_CERT` | `tls_cert` | TLS certificate file |
| `--tls_key` | `LVMSYNC_TLS_KEY` | `tls_key` | TLS key file |
| `--ca_cert` | `LVMSYNC_CA_CERT` | `ca_cert` | CA certificate file |
| `--allow_insecure` | `LVMSYNC_ALLOW_INSECURE` | `allow_insecure` | Allow insecure (no TLS) |
| `--sudo_path` | `LVMSYNC_SUDO_PATH` | `sudo_path` | Path to sudo executable |

### Control Plane Flow

1. **Handshake** – clients advertise `sector_size`, `alignment`, `max_concurrency`, and whether deduplication and compression are supported.
2. **Session Creation** – the client sends an ephemeral certificate and receives a session ID, server certificate, and pre-shared key.
3. **Resume Bitmap** – dirty block bitmaps are streamed with the session ID to resume interrupted transfers.
4. **Ack/Ping Stream** – a bidirectional stream of `Ack` messages per session provides keep-alives and progress confirmation.
5. **Finalization** – the client requests completion using the session ID when replication is done.

### `config.yaml` example

```yaml
parallel: 4               # General Options
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
lvmsync --parallel 8 --snapshot_size 10% /dev/vg0/snap0 /mnt/backup
```

With environment variables:

```sh
LVMSYNC_PARALLEL=8 LVMSYNC_SNAPSHOT_SIZE=10% lvmsync /dev/vg0/snap0 /mnt/backup
```

With a config file:

```sh
lvmsync --config config.yaml /dev/vg0/snap0 /mnt/backup
```

### Logging and progress

Logs are emitted with [zap](https://github.com/uber-go/zap) to stderr. Progress updates are also written to stderr when
`--progress` is enabled (default). Disable them with `--progress=false`.

## Installation

### Requirements

- Go 1.18+
- 64-bit Linux (x86_64) on AMD EPYC or Intel Xeon processors
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
lvmsync [options] <snapshot|lvm device> <destination>
```

The tool supports both local and remote transfers, as well as an "apply mode" for applying change dumps.

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

#### SSH Options

| Option                    | Description                                                     | Default                  |
| ------------------------- | --------------------------------------------------------------- | ------------------------ |
| `--ssh_user`              | SSH username                                                    | `"root"`                 |
| `--ssh_key`               | Path to SSH private key or use the SSH agent                    | `""`                     |
| `--ssh_port`              | SSH port number                                                 | `22`                     |
| `--known_hosts`           | Path to known_hosts file (defaults to `$HOME/.ssh/known_hosts`) | `$HOME/.ssh/known_hosts` |
| `--stricthostkeychecking` | Enable SSH StrictHostKeyChecking                                | `true`                   |

#### Remote Options

| Option                 | Description                                       | Default     |
| ---------------------- | ------------------------------------------------- | ----------- |
| `--lvmsync_path`       | Remote command to run (e.g., `"lvmsync"`)         | `"lvmsync"` |
| `--remote_pre_script`  | Remote script to run before starting the transfer | `""`        |
| `--remote_post_script` | Remote script to run after finishing the transfer | `""`        |

#### Deduplication Options

| Option               | Description                                                                                             | Default            |
| -------------------- | ------------------------------------------------------------------------------------------------------- | ------------------ |
| `--dedup`            | Deduplication mode ("fixed", "cdc", or "hybrid")                                                  | "fixed"          |
| `--cdc_min`          | Minimum chunk size for CDC                                                                              | 4096             |
| `--cdc_avg`          | Average chunk size for CDC                                                                              | 65536            |
| `--cdc_max`          | Maximum chunk size for CDC                                                                              | 1048576          |
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

| Option                     | Description                                                                                                  | Default     |
| -------------------------- | ------------------------------------------------------------------------------------------------------------ | ----------- |
| `--skip_snapshot_creation` | Skip automatic snapshot creation                                                                             | `false`     |
| `--skip_disk_check`        | Skip disk space check before snapshot creation                                                               | `false`     |
| `--snapshot_size`          | Snapshot size as an absolute value (e.g., `"20G"`) or as a percentage (e.g., `"20%"`)                        | `"20%"`     |
| `--volume_group`           | Source volume group. Derived from the source device path when empty                                          | `""`        |
| `--target_volume_group`    | Volume group name of the target LVM volume                                                                   | `""`        |
| `--target_vgs`             | Candidate target volume groups for auto-selection                                                            | `[]`        |
| `--lvm_escalation`         | Command used to re-execute the program with elevated privileges when not running as root (e.g., `"sudo -n"`) | `"sudo -n"` |

#### gRPC Options

| Option             | Description                  | Default         |
| ------------------ | ---------------------------- | --------------- |
| `--grpc-port`      | gRPC port to listen on       | `8443`          |
| `--tls-cert`       | TLS certificate file         | `""`            |
| `--tls-key`        | TLS key file                 | `""`            |
| `--ca-cert`        | CA certificate file          | `""`            |
| `--allow-insecure` | Allow insecure (disable TLS) | `false`         |
| `--sudo_path`      | Path to sudo executable      | `/usr/bin/sudo` |

### Examples

#### Local Transfer

Transfer changes from a snapshot to a destination device locally:

```sh
lvmsync /dev/vg0/snap0 /dev/vg0/data
```

#### Remote Transfer

Replicate data to a remote host (the destination is specified as `<user@host>:<device>`):

```sh
lvmsync /dev/vg0/snap0 user@remote:/dev/vg0/data
```

#### Applying Changes

Apply a change dump to a destination device (read from a file or STDIN):

```sh
lvmsync --apply dumpfile.lvm /dev/vg0/data
```

#### Using Compression

Estimate a sample of each chunk and compress only when it's worthwhile:

```sh
lvmsync --compress auto --zstd_level 2 --compress_threshold 0.85 /dev/vg0/snap0 /dev/vg0/data
```

#### Rate Limiting

Limit the transfer speed to 50MB/s:

```sh
lvmsync --speed 50MB /dev/vg0/snap0 /dev/vg0/data
```

#### Resuming a Transfer

Resume an interrupted transfer using a resume state file:

```sh
lvmsync --resume statefile /dev/vg0/snap0 /dev/vg0/data
```

#### Full LVM Operation Example

```sh
lvmsync --skip_disk_check=false --snapshot_size "25%" --volume_group "vg_data" --lvm_escalation "sudo -n" /dev/vg_data/original /dev/vg_data/destination
```

In this example, LVMSync will:

- Validate that the volume group `vg_data` exists.
- Create a snapshot of `/dev/vg_data/original` sized at 25% of the original volume.
  - Automatically re-exec with `sudo -n` if not running as root.
- Monitor snapshot usage (failing fast if usage exceeds 80%).
- Perform the block-level transfer.
- Remove the snapshot upon completion.
- Clean up gracefully if interrupted.

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
lvmsync --parallel 8 --resume statefile /dev/vg0/snap0 /dev/vg0/data
```

Environment:

```sh
LVMSYNC_PARALLEL=8 LVMSYNC_RESUME=statefile lvmsync /dev/vg0/snap0 /dev/vg0/data
```

YAML (`config.yaml`):

```yaml
parallel: 8
resume: statefile
```

#### SSH

CLI:

```sh
lvmsync --ssh-user backup --ssh-port 2222 /dev/vg0/snap0 backup:/dev/vg0/data
```

Environment:

```sh
LVMSYNC_SSH_USER=backup LVMSYNC_SSH_PORT=2222 lvmsync /dev/vg0/snap0 backup:/dev/vg0/data
```

YAML:

```yaml
ssh_user: backup
ssh_port: 2222
```

#### Remote

CLI:

```sh
lvmsync --lvmsync-path /usr/bin/lvmsync --remote-pre-script /tmp/pre.sh /dev/vg0/snap0 user@host:/dev/vg0/data
```

Environment:

```sh
LVMSYNC_LVMSYNC_PATH=/usr/bin/lvmsync LVMSYNC_REMOTE_PRE_SCRIPT=/tmp/pre.sh lvmsync /dev/vg0/snap0 user@host:/dev/vg0/data
```

YAML:

```yaml
lvmsync_path: /usr/bin/lvmsync
remote_pre_script: /tmp/pre.sh
```

#### Deduplication

CLI:

```sh
lvmsync --dedup-strategy bloom --dedup-state-file ~/.lvmsync_state /dev/vg0/snap0 /dev/vg0/data
```

Environment:

```sh
LVMSYNC_DEDUP_STRATEGY=bloom LVMSYNC_DEDUP_STATE_FILE=~/.lvmsync_state lvmsync /dev/vg0/snap0 /dev/vg0/data
```

YAML:

```yaml
dedup_strategy: bloom
dedup_state_file: ~/.lvmsync_state
```

#### Compression

LVMSync samples 8 KiB from each chunk to gauge compression efficiency. If the
compressed sample ratio is greater than or equal to `--compress_threshold`, the
chunk is sent uncompressed. In `auto` mode, chunks smaller than 256 KiB use LZ4
and larger ones select Zstd level 1 when AVX2 is available.
Levels can be tuned with `--zstd_level` (1-5) or `--lz4_level` (`fast` or `hc`).

CLI:

```sh
lvmsync --compress auto --zstd_level 2 --compress_threshold 0.85 /dev/vg0/snap0 /dev/vg0/data
```

Environment:

```sh
LVMSYNC_COMPRESS=auto LVMSYNC_ZSTD_LEVEL=2 LVMSYNC_COMPRESS_THRESHOLD=0.85 lvmsync /dev/vg0/snap0 /dev/vg0/data
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
lvmsync --snapshot-size 25% --volume-group vg_data /dev/vg_data/original /dev/vg_data/destination
```

Environment:

```sh
LVMSYNC_SNAPSHOT_SIZE=25% LVMSYNC_VOLUME_GROUP=vg_data lvmsync /dev/vg_data/original /dev/vg_data/destination
```

YAML:

```yaml
snapshot_size: "25%"
volume_group: vg_data
```

#### gRPC

`lvmsync-grpcd` accepts configuration via flags, environment variables prefixed with `LVMSYNC_GRPC_`, or a YAML file. Values are resolved in the following order: flags override environment variables, which override the config file.

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

## Credits

LVMSync is written in Go by Ofer Chen, inspired by [mpalmer/lvmsync](https://github.com/mpalmer/lvmsync).

## Contributing

Contributions are welcome. Please follow the project's coding guidelines, include appropriate logging and error handling, and update documentation as needed.

### Single-Responsibility Functions

Keep functions and packages focused on a single task to simplify maintenance and testing. Break up large components when behavior grows to preserve clarity.

### Dependency Injection

Decouple modules by injecting dependencies through interfaces or constructor parameters. This approach makes components easier to test and swap during refactoring.

### Test Coverage

Every change should include unit tests. Run `go test -cover ./...` to ensure coverage remains high and regressions are caught early.

## Development

### Development Setup

The Super-Linter workflow validates the entire repository.

### Linting

The `.golangci.yml` config uses standard Go formatters such as `gci`, `gofmt`, `gofumpt`, `goimports`, and `golines`. A misconfigured `swaggo` formatter entry was removed.
Run the linter locally to mirror CI:

```sh
golangci-lint run ./...
```

To run the tests and static checks locally:

```sh
go fmt ./...
go vet ./...
go test ./...
go test -cover ./config
```

The last command generates a coverage report for the `config` package. The same steps run in GitHub Actions under `.github/workflows/go.yml`.

## License

GPLv3 License. See the `LICENSE` file for details.
