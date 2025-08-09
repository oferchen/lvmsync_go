[![Build Status](https://github.com/oferchen/lvmsync_go/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/oferchen/lvmsync_go/actions/workflows/go.yml)
[![Build Status](https://github.com/oferchen/lvmsync_go/actions/workflows/super-linter.yml/badge.svg?branch=main)](https://github.com/oferchen/lvmsync_go/actions/workflows/super-linter.yml)
# LVMSync

LVMSync is a high-performance incremental data replication tool for LVM snapshots. It efficiently transfers only changed blocks using metadata from snapshot COW (Copy-On-Write) devices and communicates with LVM through native Go bindings rather than shell commands.

## Features

- **Incremental Block-Level Synchronization**: Transfers only changed blocks.
- **Zero-Copy Transfers**: Utilizes `splice()` for efficient data movement.
- **Parallel Execution**: Configurable concurrency for optimal performance.
- **Rate-Limiting**: Control bandwidth usage during transfers.
- **Compression**: Supports LZ4 and Zstd (with configurable compression levels and an auto mode based on CPU features).
- **Checksum Verification**: Ensures data integrity using SHA-256 or BLAKE3.
- **Native LVM2 Integration**: Uses Go bindings to `liblvm2cmd` instead of shelling out.
- **Deduplication Strategies**: Detect unchanged blocks using checksum, rolling hash, or Bloom filter with persistent state.
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
- **Configuration Validation**: Checks key parameters (e.g., volume group existence, escalation command) before starting operations.

## Architecture

LVMSync is organized into modular packages to keep concerns separated:

- `lvm` – manages snapshot creation, monitoring, and cleanup.
- `transfer` – performs block-level synchronization, compression, deduplication, and resume logic.
- `remote` – wraps SSH functionality for running commands on remote hosts and coordinating transfers.
- `config` – parses and validates configuration files and CLI options.
- `common` and `internal` – shared helpers and internal utilities such as multi-error handling.

This structure allows individual packages to be developed and tested in isolation.

## Logging

LVMSync emits structured logs using [zap](https://github.com/uber-go/zap). Errors are logged with
structured fields instead of being written to stderr, and the logger is flushed on shutdown to
ensure all entries are persisted.

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

| Option               | Description                                                                                                      | Default            |
| -------------------- | ---------------------------------------------------------------------------------------------------------------- | ------------------ |
| `--dedup_strategy`   | Deduplication strategy (`"none"`, `"auto"`, `"checksum"`, `"rolling_hash"`, or `"bloom"`); use `none` to disable | `"none"`           |
| `--dedup_state_file` | Path to deduplication state file                                                                                 | `~/.lvmsync_dedup` |
| `--bloom_entries`    | Estimated number of entries for bloom filter                                                                     | `1000000`          |
| `--bloom_fp_rate`    | False positive rate for bloom filter                                                                             | `0.01`             |

#### Compression Options

| Option                   | Description                                                       | Default  |
| ------------------------ | ----------------------------------------------------------------- | -------- |
| `--compress`             | Compression type (options: `"none"`, `"lz4"`, `"zstd"`, `"auto"`) | `"auto"` |
| `--compress_level`       | Compression level for Zstd (ignored for LZ4)                      | `3`      |
| `--compress_concurrency` | Number of goroutines used for compression (`0` to use all cores)  | `0`      |

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
| `--grpc_port`      | gRPC port to listen on       | `8443`          |
| `--tls_cert`       | TLS certificate file         | `""`            |
| `--tls_key`        | TLS key file                 | `""`            |
| `--ca_cert`        | CA certificate file          | `""`            |
| `--allow_insecure` | Allow insecure (disable TLS) | `true`          |
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

Enable Zstd compression with a specified compression level:

```sh
lvmsync --compress zstd --compress_level 3 /dev/vg0/snap0 /dev/vg0/data
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

CLI:

```sh
lvmsync --compress zstd --compress-level 5 /dev/vg0/snap0 /dev/vg0/data
```

Environment:

```sh
LVMSYNC_COMPRESS=zstd LVMSYNC_COMPRESS_LEVEL=5 lvmsync /dev/vg0/snap0 /dev/vg0/data
```

YAML:

```yaml
compress: zstd
compress_level: 5
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

CLI:

```sh
lvmsync-grpcd --grpc_port 9443
```

Environment:

```sh
LVMSYNC_GRPC_PORT=9443 lvmsync-grpcd
```

YAML:

```yaml
grpc_port: 9443
```

## Configuration File (`config.yaml`)

You can supply configuration via a YAML file. Below is an example configuration file covering all options:

```yaml
# config.yaml - LVMSync default configuration

config: "" # Optional: specify an alternative configuration file path

apply: "" # Apply mode: read change dump from file ('-' for STDIN) and apply to destination device
stdout: false # Write change dump to STDOUT
parallel: 4 # Number of concurrent workers
zerocopy: false # Enable zero-copy transfers (only in sequential mode)
max_retries: 3 # Maximum number of retries per block
resume: "" # Path to resume state file
speed: "100MB" # Transfer speed limit (e.g., "100MB")
verbose: 0 # Verbosity level
verify_checksum: false # Enable checksum verification
progress: true # Show progress percentage during the transfer

# Deduplication Options:
dedup_strategy: "none" # Strategy: "none", "auto", "checksum", "rolling_hash", or "bloom" (use "none" to disable)
dedup_state_file: "~/.lvmsync_dedup" # Path to deduplication state file
bloom_entries: 1000000 # Estimated number of entries for bloom filter
bloom_fp_rate: 0.01 # False positive rate for bloom filter

# SSH Options:
ssh_user: "root" # SSH username
ssh_key: "" # Path to SSH private key or use SSH agent
ssh_port: 22 # SSH port
known_hosts: "~/.ssh/known_hosts" # Path to known_hosts file
stricthostkeychecking: true # Enable SSH StrictHostKeyChecking

# Remote Options:
lvmsync_path: "lvmsync" # Remote command to run
remote_pre_script: "" # Remote script to run before starting transfer
remote_post_script: "" # Remote script to run after finishing transfer

# Compression Options:
compress: "auto" # Compression type (options: "none", "lz4", "zstd", "auto")
compress_level: 3 # Compression level for zstd (ignored for lz4)
compress_concurrency: 0 # Number of goroutines for compression (0 to use all cores)

# LVM Options:
skip_snapshot_creation: false # Skip automatic snapshot creation
skip_disk_check: false # Skip disk space check before snapshot creation
snapshot_size: "20%" # Snapshot size as an absolute value (e.g., "20G") or as a percentage (e.g., "20%")
volume_group: "" # Source volume group. Derived from the source path when empty
target_volume_group: "" # Volume group name of the target LVM volume
target_vgs: [] # Candidate target VGs for auto-selection
lvm_escalation: "sudo -n" # Command used to re-execute the program with elevated privileges when needed
```

LVMSync installs signal handlers for SIGINT and SIGTERM. If an interruption occurs, the tool will attempt to remove any created snapshot before exiting, ensuring no orphaned snapshots remain.

## Configuration Validation

Before starting, LVMSync validates key configuration parameters:

- Verifies that the specified volume groups exist.
- Ensures the escalation command is available if not running as root.

Invalid configurations will cause the tool to abort with a clear error message.

## Credits

LVMSync is written in Go by Ofer Chen, inspired by [mpalmer/lvmsync](https://github.com/mpalmer/lvmsync).

## Contributing

Contributions are welcome. Please follow the project's coding guidelines, include appropriate logging and error handling, and update documentation as needed.

## Development

### Development Setup

The Super-Linter workflow validates the entire repository and runs `golangci-lint run ./...` from the repository root.
Run the same command locally to mirror CI:

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
